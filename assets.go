package main

import (
	"bytes"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

func (cfg apiConfig) ensureAssetsDir() error {
	if _, err := os.Stat(cfg.assetsRoot); os.IsNotExist(err) {
		return os.Mkdir(cfg.assetsRoot, 0755)
	}
	return nil
}

func getAssetPath(mediaType string) string {
	ext := mediaTypeToExt(mediaType)
	base := make([]byte, 32)
	_, err := rand.Read(base)
	if err != nil {
		panic("failed to generate random bytes")
	}
	id := base64.RawURLEncoding.EncodeToString(base)
	return fmt.Sprintf("%s%s", id, ext)
}

func (cfg apiConfig) getAssetDiskPath(assetPath string) string {
	return filepath.Join(cfg.assetsRoot, assetPath)
}

func (cfg apiConfig) getAssetURL(assetPath string) string {
	return fmt.Sprintf("http://localhost:%s/assets/%s", cfg.port, assetPath)
}

func mediaTypeToExt(mediaType string) string {
	parts := strings.Split(mediaType, "/")
	if len(parts) != 2 {
		return ".bin"
	}
	return "." + parts[1]
}

func getVideoAspectRatio(filePath string) (string, error) {
	var output bytes.Buffer
	ffprobeCmd := exec.Command("ffprobe", "-v", "error", "-print_format", "json", "-show_streams", filePath)
	ffprobeCmd.Stdout = &output
	err := ffprobeCmd.Run()
	if err != nil {
		return "", err
	}

	var result struct {
		Streams []struct {
			DisplayAspectRatio string `json:"display_aspect_ratio"`
		} `json:"streams"`
	}
	err = json.Unmarshal(output.Bytes(), &result)
	if err != nil {
		return "", err
	}

	if len(result.Streams) == 0 {
		return "", errors.New("no video streams found")
	}
	displayAspectRatio := result.Streams[0].DisplayAspectRatio

	if displayAspectRatio == "16:9" || displayAspectRatio == "9:16" {
		return displayAspectRatio, nil
	}
	return "other", nil
}

func processVideoForFastStart(filePath string) (string, error) {
	outputPath := filePath + ".processing"
	ffmpegCmd := exec.Command("ffmpeg",
		"-i", filePath,
		"-c", "copy",
		"-movflags", "faststart",
		"-f", "mp4",
		outputPath)

	err := ffmpegCmd.Run()
	if err != nil {
		return "", err
	}

	fileInfo, err := os.Stat(outputPath)
	if err != nil {
		return "", fmt.Errorf("could not stat processed file: %v", err)
	}
	if fileInfo.Size() == 0 {
		return "", fmt.Errorf("processed file is empty")
	}

	return outputPath, nil
}
