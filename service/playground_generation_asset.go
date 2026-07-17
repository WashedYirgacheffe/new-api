package service

import (
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/model"
)

const (
	PlaygroundAssetDirectoryEnv = "PLAYGROUND_ASSET_DIR"
	PlaygroundAssetRequestBytes = (model.PlaygroundGenerationMaximumAssetBytes*4)/3 + 1024*1024
)

var ErrPlaygroundAssetStorageUnavailable = errors.New("playground asset storage is not configured")

type PlaygroundGenerationAssetResult struct {
	Ordinal   int    `json:"ordinal"`
	URL       string `json:"url"`
	MimeType  string `json:"mime_type"`
	SizeBytes int64  `json:"size_bytes"`
}

func playgroundAssetRoot() (string, error) {
	root := strings.TrimSpace(os.Getenv(PlaygroundAssetDirectoryEnv))
	if root == "" {
		return "", ErrPlaygroundAssetStorageUnavailable
	}
	abs, err := filepath.Abs(root)
	if err != nil {
		return "", fmt.Errorf("resolve playground asset directory: %w", err)
	}
	return abs, nil
}

func decodePlaygroundImageDataURL(value string) ([]byte, string, string, error) {
	header, encoded, ok := strings.Cut(strings.TrimSpace(value), ",")
	normalizedHeader := strings.ToLower(header)
	if !ok || !strings.HasPrefix(normalizedHeader, "data:image/") || !strings.HasSuffix(normalizedHeader, ";base64") {
		return nil, "", "", errors.New("asset must be a base64 image data URL")
	}
	mimeType := strings.TrimSuffix(strings.TrimPrefix(normalizedHeader, "data:"), ";base64")
	if mimeType == "image/jpg" {
		mimeType = "image/jpeg"
	}
	extensions := map[string]string{
		"image/png":  ".png",
		"image/jpeg": ".jpg",
		"image/webp": ".webp",
		"image/gif":  ".gif",
	}
	extension, ok := extensions[mimeType]
	if !ok {
		return nil, "", "", fmt.Errorf("unsupported playground image MIME type %q", mimeType)
	}
	data, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return nil, "", "", errors.New("asset contains invalid base64 data")
	}
	if len(data) == 0 || len(data) > model.PlaygroundGenerationMaximumAssetBytes {
		return nil, "", "", fmt.Errorf("asset must be between 1 and %d bytes", model.PlaygroundGenerationMaximumAssetBytes)
	}
	detected := http.DetectContentType(data)
	if detected != mimeType {
		return nil, "", "", fmt.Errorf("asset content type %q does not match %q", detected, mimeType)
	}
	return data, mimeType, extension, nil
}

func publishPlaygroundAssetFile(absolutePath string, data []byte, digest string) (bool, error) {
	directory := filepath.Dir(absolutePath)
	if err := os.MkdirAll(directory, 0o750); err != nil {
		return false, fmt.Errorf("create playground asset directory: %w", err)
	}
	temporary, err := os.CreateTemp(directory, ".playground-asset-*")
	if err != nil {
		return false, fmt.Errorf("create temporary playground asset: %w", err)
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	if err := temporary.Chmod(0o640); err != nil {
		_ = temporary.Close()
		return false, fmt.Errorf("set playground asset permissions: %w", err)
	}
	if _, err := temporary.Write(data); err != nil {
		_ = temporary.Close()
		return false, fmt.Errorf("write playground asset: %w", err)
	}
	if err := temporary.Sync(); err != nil {
		_ = temporary.Close()
		return false, fmt.Errorf("sync playground asset: %w", err)
	}
	if err := temporary.Close(); err != nil {
		return false, fmt.Errorf("close playground asset: %w", err)
	}
	if err := os.Link(temporaryPath, absolutePath); err == nil {
		return true, nil
	} else if !errors.Is(err, os.ErrExist) {
		return false, fmt.Errorf("publish playground asset: %w", err)
	}

	existing, err := os.Open(absolutePath)
	if err != nil {
		return false, fmt.Errorf("open existing playground asset: %w", err)
	}
	hasher := sha256.New()
	written, copyErr := io.Copy(hasher, existing)
	closeErr := existing.Close()
	if copyErr != nil {
		return false, fmt.Errorf("verify existing playground asset: %w", copyErr)
	}
	if closeErr != nil {
		return false, fmt.Errorf("close existing playground asset: %w", closeErr)
	}
	if written != int64(len(data)) || fmt.Sprintf("%x", hasher.Sum(nil)) != digest {
		return false, errors.New("existing playground asset does not match uploaded content")
	}
	return false, nil
}

func PersistPlaygroundGenerationAsset(userId int, generationId string, ordinal int, dataURL string) (*PlaygroundGenerationAssetResult, error) {
	root, err := playgroundAssetRoot()
	if err != nil {
		return nil, err
	}
	generation, err := model.GetPlaygroundGenerationById(userId, generationId)
	if err != nil {
		return nil, err
	}
	if generation.Status != model.PlaygroundGenerationStatusPending {
		return nil, model.ErrPlaygroundGenerationFinalized
	}
	generationId = generation.Id
	data, mimeType, extension, err := decodePlaygroundImageDataURL(dataURL)
	if err != nil {
		return nil, err
	}
	digest := fmt.Sprintf("%x", sha256.Sum256(data))
	relativePath := filepath.Join(strconv.Itoa(userId), generationId, fmt.Sprintf("%02d-%s%s", ordinal, digest, extension))
	absolutePath := filepath.Join(root, relativePath)
	createdFile, err := publishPlaygroundAssetFile(absolutePath, data, digest)
	if err != nil {
		return nil, err
	}

	asset, err := model.SavePlaygroundGenerationAsset(userId, model.PlaygroundGenerationAssetCreate{
		GenerationId: generationId,
		Ordinal:      ordinal,
		MimeType:     mimeType,
		SizeBytes:    int64(len(data)),
		SHA256:       digest,
		RelativePath: filepath.ToSlash(relativePath),
	})
	if err != nil {
		if createdFile {
			existing, lookupErr := model.GetPlaygroundGenerationAsset(userId, generationId, ordinal)
			if errors.Is(lookupErr, model.ErrPlaygroundGenerationAssetNotFound) ||
				(lookupErr == nil && existing.SHA256 != digest) {
				_ = os.Remove(absolutePath)
			}
		}
		return nil, err
	}
	return &PlaygroundGenerationAssetResult{
		Ordinal:   asset.Ordinal,
		URL:       fmt.Sprintf("/pg/generations/%s/assets/%d", asset.GenerationId, asset.Ordinal),
		MimeType:  asset.MimeType,
		SizeBytes: asset.SizeBytes,
	}, nil
}

func ResolvePlaygroundGenerationAsset(userId int, generationId string, ordinal int) (*model.PlaygroundGenerationAsset, string, error) {
	root, err := playgroundAssetRoot()
	if err != nil {
		return nil, "", err
	}
	asset, err := model.GetPlaygroundGenerationAsset(userId, generationId, ordinal)
	if err != nil {
		return nil, "", err
	}
	absolutePath := filepath.Join(root, filepath.FromSlash(asset.RelativePath))
	relative, err := filepath.Rel(root, absolutePath)
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return nil, "", errors.New("playground asset path escapes configured directory")
	}
	if _, err := os.Stat(absolutePath); err != nil {
		return nil, "", fmt.Errorf("read playground asset: %w", err)
	}
	return asset, absolutePath, nil
}

func DeletePlaygroundGenerationHistory(userId int, generationId string) error {
	generation, err := model.GetPlaygroundGenerationById(userId, generationId)
	if err != nil {
		return err
	}
	generationId = generation.Id
	assetCount, err := model.CountPlaygroundGenerationAssets(userId, generationId)
	if err != nil {
		return err
	}
	root, rootErr := playgroundAssetRoot()
	if rootErr != nil {
		if !errors.Is(rootErr, ErrPlaygroundAssetStorageUnavailable) || assetCount > 0 {
			return rootErr
		}
	} else {
		generationDirectory := filepath.Join(root, strconv.Itoa(userId), generationId)
		if relative, err := filepath.Rel(root, generationDirectory); err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
			return errors.New("playground generation directory escapes configured asset directory")
		}
		if err := os.RemoveAll(generationDirectory); err != nil {
			return fmt.Errorf("delete playground generation assets: %w", err)
		}
	}
	return model.DeletePlaygroundGeneration(userId, generationId)
}
