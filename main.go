package main

import (
	"crypto/md5"
	"errors"
	"flag"
	"fmt"
	"github.com/avast/retry-go/v4"
	"github.com/upyun/go-sdk/v3/upyun"
	"io/ioutil"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

const retryDelay = time.Millisecond * 500

type UpYunDeployer struct {
	up         *upyun.UpYun
	localDir   string
	publishDir string
}

func (d *UpYunDeployer) GetAllRemoteFiles() (map[string]int, map[string]int) {
	return d.GetAllRemoteFilesByPath("/")
}

func (d *UpYunDeployer) GetAllRemoteFilesByPath(path string) (map[string]int, map[string]int) {
	objsChan := make(chan *upyun.FileInfo)

	go func() {
		objsChan <- &upyun.FileInfo{
			Name:  path,
			IsDir: true,
		}
	}()

	counter := 0

	files := make(map[string]int)
	directories := make(map[string]int)

	for obj := range objsChan {
		if obj == nil {
			counter--
			if counter == 0 {
				break
			}
			continue
		}

		depth := 1
		if obj.Name != "/" {
			obj.Name = strings.Trim(obj.Name, "/")
			depth = len(strings.Split(obj.Name, "/"))
		}

		if !obj.IsDir {
			files[obj.Name] = depth
			continue
		}

		directories[obj.Name] = depth
		go d.listDirs(obj.Name, objsChan)
		counter++
	}

	return files, directories
}

func (d *UpYunDeployer) UploadFiles() {
	publishDir := d.publishDir
	if len(publishDir) == 0 {
		publishDir = "/"
	}

	files, dirs := d.GetAllRemoteFilesByPath(publishDir)
	existsFileCount := len(files)
	uploadedFileCount := 0

	var urls []string
	wg := &sync.WaitGroup{}

	err := filepath.Walk(d.localDir, func(filename string, file os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		relativeFilename := strings.Trim(strings.ReplaceAll(filename, d.localDir, ""), "/")

		if file.IsDir() || strings.HasPrefix(file.Name(), ".") || strings.HasPrefix(relativeFilename, ".") {
			fmt.Printf("[%s/%s] skiped!\n", d.publishDir, relativeFilename)
			return nil
		}

		relativeFilename = filepath.Join(d.publishDir, relativeFilename)

		delete(files, relativeFilename)
		urls = append(urls, relativeFilename)

		str := ""
		segments := strings.Split(strings.Trim(strings.ReplaceAll(relativeFilename, file.Name(), ""), "/"), "/")
		for _, segment := range segments {
			str = filepath.Join(str, segment)
			if _, ok := dirs[str]; ok {
				delete(dirs, str)
			}
		}

		go d.handleFile(wg, filename, relativeFilename)

		uploadedFileCount++

		return nil
	})

	wg.Wait()

	fmt.Printf("exists: %d, uploaded: %d\n", existsFileCount, uploadedFileCount)

	if err != nil {
		fmt.Printf("file walk err: %s\n", err)
		return
	}

	failUrls, err := d.up.Purge(urls)
	if err != nil {
		fmt.Printf("purge failed urls: %v, err: %s\n", failUrls, err)
	}

	delete(dirs, "/")

	d.deleteFiles(files)
	d.deleteDirs(dirs)
}

func (d *UpYunDeployer) deleteFiles(files map[string]int) {
	fmt.Println("deleting files...")
	ch := make(chan string, 20)
	go func() {
		for file := range files {
			ch <- file
		}
		ch <- ""
	}()

	for file := range ch {
		if len(file) == 0 {
			break
		}
		go func(f string) {
			err := d.deleteFile(f, true)
			if err != nil {
				fmt.Printf("[%s] delete failed: %v\n", f, err)
				return
			}
			fmt.Printf("[%s] deleted!\n", f)
		}(file)
	}
}

func (d *UpYunDeployer) deleteDirs(dirs map[string]int) {
	fmt.Println("deleting dirs...")

	maxDepth := 0
	for _, depth := range dirs {
		if depth > maxDepth {
			maxDepth = depth
		}
	}

	sortedDirs := make([]string, 0)
	for maxDepth > 0 {
		for dir, depth := range dirs {
			if depth == maxDepth {
				sortedDirs = append(sortedDirs, dir)
			}
		}
		maxDepth--
	}

	for _, dir := range sortedDirs {
		err := d.deleteFile(dir, true)
		if err != nil {
			fmt.Printf("[%s] delete failed: %v", dir, err)
			continue
		}
		fmt.Printf("[%s] deleted!\n", dir)
	}
}

func (d *UpYunDeployer) handleFile(wg *sync.WaitGroup, filename string, relativeFilename string) {
	wg.Add(1)
	defer wg.Done()

	data, err := ioutil.ReadFile(filename)
	if err != nil {
		fmt.Printf("[%s] read file failed, reason: %s\n", relativeFilename, err)
		return
	}

	contentType := detectContentType(filename, data)

	remoteFileInfo, err := d.getFileInfo(relativeFilename)
	putObjectConfig := &upyun.PutObjectConfig{
		Path:      relativeFilename,
		LocalPath: filename,
		Headers: map[string]string{
			"Content-Type": contentType,
		},
	}

	if upyun.IsNotExist(err) {
		d.uploadFile(putObjectConfig, false)
		return
	}

	if err == nil {
		if remoteFileInfo.ContentType == contentType && remoteFileInfo.MD5 == fmt.Sprintf("%x", md5.Sum(data)) {
			fmt.Printf("[%s] cached!\n", relativeFilename)
			return
		}
		err = d.deleteFile(relativeFilename, true)
		if err != nil {
			fmt.Printf("[%s] failed to delete before uploading: %v\n", relativeFilename, err)
			return
		}
	} else {
		fmt.Printf("[%s] get file info failed: %v\n", relativeFilename, err)
	}

	d.uploadFile(putObjectConfig, true)
}

func (d *UpYunDeployer) getFileInfo(filename string) (*upyun.FileInfo, error) {
	var err error
	var fileInfo *upyun.FileInfo

	_ = retry.Do(
		func() error {
			fileInfo, err = d.up.GetInfo(filename)
			return err
		},
		retry.RetryIf(func(err error) bool {
			return !upyun.IsNotExist(err)
		}),
		retry.OnRetry(func(n uint, err error) {
			fmt.Printf("[%s] retrying to get file info\n", filename)
		}),
		retry.Delay(retryDelay),
		retry.Attempts(3),
	)

	return fileInfo, err
}

func (d *UpYunDeployer) uploadFile(putObjectConfig *upyun.PutObjectConfig, refresh bool) {
	err := retry.Do(
		func() error {
			return d.up.Put(putObjectConfig)
		},
		retry.OnRetry(func(n uint, err error) {
			fmt.Printf("[%s] retrying to upload file info\n", putObjectConfig.Path)
		}),
		retry.Delay(retryDelay),
		retry.Attempts(3),
	)

	action := "upload"
	if refresh {
		action = "refresh"
	}

	if err == nil {
		fmt.Printf("[%s] %sed!\n", putObjectConfig.Path, action)
	} else {
		fmt.Printf("[%s] %s failed: %v\n", putObjectConfig.Path, action, errors.Unwrap(err))
	}
}

func (d *UpYunDeployer) deleteFile(path string, async bool) error {
	err := retry.Do(
		func() error {
			return d.up.Delete(&upyun.DeleteObjectConfig{
				Path:  path,
				Async: async,
			})
		},
		retry.RetryIf(func(err error) bool {
			return !upyun.IsNotExist(err)
		}),
		retry.OnRetry(func(n uint, err error) {
			fmt.Printf("[%s] retrying to delete file info\n", path)
		}),
		retry.Delay(retryDelay),
		retry.Attempts(3),
	)

	err = errors.Unwrap(err)

	if upyun.IsNotExist(err) {
		fmt.Printf("[%s] file does not exist when deleting", path)
		return nil
	}

	return err
}

func (d UpYunDeployer) listDirs(path string, ch chan *upyun.FileInfo) {
	objsChan := make(chan *upyun.FileInfo)
	go func() {
		err := d.up.List(&upyun.GetObjectsConfig{
			Path:        path,
			ObjectsChan: objsChan,
			Headers: map[string]string{
				"X-List-Limit": "10000",
			},
		})
		if err != nil {
			fmt.Printf("[%s] list dirs failed: %v\n", path, err)
		}
	}()

	for obj := range objsChan {
		obj.Name = filepath.Join(path, obj.Name)
		ch <- obj
	}
	ch <- nil
}

func detectContentType(filename string, data []byte) string {
	ext := filepath.Ext(filename)
	if len(ext) > 0 {
		return mime.TypeByExtension(ext)
	}
	return http.DetectContentType(data)
}

var bucket = flag.String("bucket", "", "")
var operator = flag.String("operator", "", "")
var password = flag.String("password", "", "")
var localDir = flag.String("local_dir", "", "")
var publishDir = flag.String("publish_dir", "", "")

func main() {
	flag.Parse()

	up := upyun.NewUpYun(&upyun.UpYunConfig{
		Bucket:   *bucket,
		Operator: *operator,
		Password: *password,
	})

	deployer := &UpYunDeployer{
		up:         up,
		localDir:   strings.TrimPrefix(*localDir, "./"),
		publishDir: strings.Trim(*publishDir, "./"),
	}

	begin := time.Now()

	deployer.UploadFiles()

	fmt.Printf("done! consumed: %s\n", time.Since(begin))
}
