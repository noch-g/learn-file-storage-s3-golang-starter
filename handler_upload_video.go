package main

import (
	"context"
	"fmt"
	"io"
	"mime"
	"net/http"
	"os"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/noch-g/learn-file-storage-s3-golang-starter/internal/auth"
	"github.com/google/uuid"
)

func (cfg *apiConfig) handlerUploadVideo(w http.ResponseWriter, r *http.Request) {
	videoIDString := r.PathValue("videoID")
	videoID, err := uuid.Parse(videoIDString)
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Invalid ID", err)
		return
	}

	token, err := auth.GetBearerToken(r.Header)
	if err != nil {
		respondWithError(w, http.StatusUnauthorized, "Couldn't find JWT", err)
		return
	}

	userID, err := auth.ValidateJWT(token, cfg.jwtSecret)
	if err != nil {
		respondWithError(w, http.StatusUnauthorized, "Couldn't validate JWT", err)
		return
	}

	video, err := cfg.db.GetVideo(videoID)
	if err != nil {
		respondWithError(w, http.StatusNotFound, "Video not found", err)
		return
	}

	if video.UserID != userID {
		respondWithError(w, http.StatusForbidden, "User does not have permission to upload video", nil)
		return
	}

	const maxMemory = 10 << 30
	r.ParseMultipartForm(maxMemory)

	multiPartFile, header, err := r.FormFile("video")
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Unable to parse form file", err)
		return
	}
	defer multiPartFile.Close()

	mediaType, _, err := mime.ParseMediaType(header.Header.Get("Content-Type"))
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Invalid Content-Type for thumbnail", err)
		return
	}
	if mediaType != "video/mp4" {
		respondWithError(w, http.StatusBadRequest, "Only accepted video type is mp4", nil)
		return
	}

	tempFile, err := os.CreateTemp("", "tubely-upload.mp4")
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Couldn't create temp video file", err)
		return
	}
	defer os.Remove(tempFile.Name())
	defer tempFile.Close()

	_, err = io.Copy(tempFile, multiPartFile)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Couldn't write video to file", err)
		return
	}

	outputPath, err := processVideoForFastStart(tempFile.Name())
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Couldn't process video for fast start", err)
		return
	}
	processedFile, err := os.Open(outputPath)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Couldn't open processed video", err)
		return
	}
	defer os.Remove(outputPath)
	defer processedFile.Close()

	aspectRatio, err := getVideoAspectRatio(outputPath)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Couldn't get video aspect ratio", err)
		return
	}

	var key string
	switch aspectRatio {
	case "16:9":
		key = "landscape/" + getAssetPath(mediaType)
	case "9:16":
		key = "portrait/" + getAssetPath(mediaType)
	default:
		key = "other/" + getAssetPath(mediaType)
	}

	_, err = cfg.s3Client.PutObject(context.TODO(), &s3.PutObjectInput{
		Bucket:      aws.String(cfg.s3Bucket),
		Key:         aws.String(key),
		Body:        processedFile,
		ContentType: aws.String(mediaType),
	})
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Couldn't upload video to S3", err)
		return
	}

	url := fmt.Sprintf("https://%s/%s", cfg.s3CfDistribution, key)
	video.VideoURL = &url

	err = cfg.db.UpdateVideo(video)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Couldn't update video", err)
		return
	}

	respondWithJSON(w, http.StatusOK, video)
}

// func generatePresignedURL(s3Client *s3.Client, bucket, key string, expireTime time.Duration) (string, error) {
// 	presignedClient := s3.NewPresignClient(s3Client)
// 	req, err := presignedClient.PresignGetObject(context.TODO(), &s3.GetObjectInput{
// 		Bucket: aws.String(bucket),
// 		Key:    aws.String(key),
// 	}, s3.WithPresignExpires(expireTime))
// 	if err != nil {
// 		return "", err
// 	}
// 	return req.URL, nil
// }

// func (cfg *apiConfig) dbVideoToSignedVideo(video database.Video) (database.Video, error) {
// 	if video.VideoURL == nil {
// 		return video, nil
// 	}

// 	parts := strings.Split(*video.VideoURL, ",")
// 	if len(parts) != 2 {
// 		return database.Video{}, fmt.Errorf("invalid video URL")
// 	}

// 	bucket := parts[0]
// 	key := parts[1]

// 	presignedURL, err := generatePresignedURL(cfg.s3Client, bucket, key, 15*time.Minute)
// 	if err != nil {
// 		return database.Video{}, err
// 	}

// 	video.VideoURL = &presignedURL
// 	return video, nil
// }
