package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	yadisk "github.com/g-konst/yadisk-go/pkg"
)

var (
	flagSet      = flag.NewFlagSet(os.Args[0], flag.ExitOnError)
	fromDir      = flagSet.String("i", ".", "Path on local")
	toDir        = flagSet.String("o", "disk:/", "Path on Yandex Disk")
	yandex_token = flagSet.String("token", os.Getenv("YANDEX_DISK_TOKEN"), "Yandex Disk OAuth token")
	workers      = flagSet.Int("w", 1, "Workers count")
	retries      = flagSet.Int("r", 1, "Max attempt count")
)

func main() {
	flagSet.Parse(os.Args[1:])

	if *yandex_token == "" {
		panic("YANDEX_DISK_TOKEN is not set")
	}

	client := yadisk.NewYandexDiskClient(*yandex_token)
	ctx := context.Background()

	_, _, err := client.Disk.Get(ctx, nil)
	if err != nil {
		panic(err)
	}

	var wg sync.WaitGroup
	semaphore := make(chan struct{}, *workers)
	err = filepath.Walk(*fromDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		relPath := strings.TrimPrefix(path, *fromDir)
		remotePath := filepath.ToSlash(filepath.Join(*toDir, relPath))

		if info.IsDir() {
			fmt.Println("Create directory:", remotePath)
			_, _, err := client.Resources.Create(ctx, yadisk.ResourcesCreateOpts{
				Path: remotePath,
			})
			if err != nil {
				fmt.Println("Error creating directory:", err)
				return nil
			}
		} else {
			wg.Add(1)
			semaphore <- struct{}{}
			go UploadFile(ctx, &wg, remotePath, path, client, semaphore)
		}
		return nil
	})

	if err != nil {
		fmt.Println("Error walking directory:", err)
		return
	}

	wg.Wait()
	fmt.Println("Done!")
}

func UploadFile(
	ctx context.Context,
	wg *sync.WaitGroup,
	remotePath string,
	path string,
	client *yadisk.YandexDiskClient,
	semaphore chan struct{},
) {
	defer wg.Done()

	fmt.Println("Start uploading file:", path)

	// Yandex Disk throttle speed on some types of files (mp4, zip, etc).
	// Add ".tmp" extension to prevent Yandex Disk from throttling.
	tmpName := remotePath + ".tmp"
	link, _, err := client.Resources.GetUploadLink(
		ctx,
		yadisk.ResourcesGetUploadLinkOpts{
			Path:      tmpName,
			Overwrite: true,
		},
	)
	if err != nil {
		fmt.Println("Error getting res info:", err)
		return
	}

	// Upload file
	err = uploadFileStream(ctx, path, link.Href, *retries, client)
	if err != nil {
		fmt.Println("Error uploading file:", err)
		return
	}

	// Rename file to original name.
	_, _, err = client.Resources.Move(ctx, yadisk.ResourcesMoveOpts{
		From:      tmpName,
		Path:      remotePath,
		Overwrite: true,
	})
	if err != nil {
		fmt.Println("Error getting res info:", err)
		return
	}

	<-semaphore
}

func uploadFileStream(
	ctx context.Context,
	filePath string,
	uploadUrl string,
	maxAttempt int,
	client *yadisk.YandexDiskClient,
) error {
	file, err := os.Open(filePath)
	if err != nil {
		fmt.Println("Error opening file:", err)
		return err
	}
	defer file.Close()

	fileInfo, err := file.Stat()
	if err != nil {
		fmt.Println("Error getting file info:", err)
		return err
	}
	fileSize := fileInfo.Size()

	url, err := url.Parse(uploadUrl)
	if err != nil {
		fmt.Println("Error parsing upload URL:", err)
		return err
	}

	for i := 0; i < maxAttempt; i++ {
		offset, err := client.Resources.GetUploadedBytes(ctx, uploadUrl)
		if err != nil {
			fmt.Println("Error getting uploaded bytes:", err)
			return err
		}
		_, err = file.Seek(offset, io.SeekStart)
		if err != nil {
			fmt.Println("Error seeking file:", err)
			return err
		}

		req, err := client.NewRequest(ctx, "PUT", url, file, nil)
		if err != nil {
			fmt.Println("Error creating request:", err)
			return err
		}
		req.Header.Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", offset, fileSize-1, fileSize))
		req.Header.Set("Content-Type", "application/octet-stream")
		req.Header.Set("Content-Length", fmt.Sprintf("%d", fileSize))

		resp, err := client.Do(req, nil)
		if err != nil || resp.StatusCode != http.StatusCreated {
			if err != nil {
				fmt.Println("Error uploading file:", err)
			} else {
				body, _ := io.ReadAll(resp.Body)
				fmt.Println("Upload Failed:", body)
			}

			time.Sleep(time.Second * time.Duration(i*2))
			continue
		}
		fmt.Println("File Uploaded: ", uploadUrl)
		return nil
	}
	return fmt.Errorf("error: max attempt reached")
}
