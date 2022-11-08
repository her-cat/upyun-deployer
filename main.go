package main

import (
	"crypto/md5"
	"flag"
	"fmt"
	"github.com/upyun/go-sdk/v3/upyun"
	"io/ioutil"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

type UpYunDeployer struct {
	up       *upyun.UpYun
	basicDir string
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

		if obj.Name != "/" {
			obj.Name = strings.Trim(obj.Name, "/")
		}

		if !obj.IsDir {
			files[obj.Name] = 1
			continue
		}

		directories[obj.Name] = 1
		go d.listDirs(obj.Name, objsChan)
		counter++
	}

	return files, directories
}

func (d *UpYunDeployer) UploadFiles() {
	files, dirs := d.GetAllRemoteFiles()

	var urls []string
	wg := &sync.WaitGroup{}

	_ = filepath.Walk(d.basicDir, func(filename string, file os.FileInfo, err error) error {
		relativeFilename := strings.ReplaceAll(filename, d.basicDir, "")
		if file.IsDir() || strings.HasPrefix(file.Name(), ".") || strings.HasPrefix(relativeFilename, ".") {
			return nil
		}

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

		return nil
	})

	wg.Wait()

	failUrls, err := d.up.Purge(urls)
	if err != nil {
		fmt.Printf("purge failed urls: %v, err: %s\n", failUrls, err)
	}

	d.deleteFiles(files)
	d.deleteDirs(dirs)
}

func (d *UpYunDeployer) deleteFiles(files map[string]int) {
	fmt.Println("deleting files...")
	for file := range files {
		_ = d.up.Delete(&upyun.DeleteObjectConfig{
			Path:  file,
			Async: true,
		})
		fmt.Printf("[%s] deleted!\n", file)
	}
}

func (d *UpYunDeployer) deleteDirs(dirs map[string]int) {
	fmt.Println("deleting dirs...")
	wg := sync.WaitGroup{}
	for dir := range dirs {
		go func(dir string) {
			wg.Add(1)
			defer wg.Done()
			_ = d.up.Delete(&upyun.DeleteObjectConfig{
				Path: dir,
			})
			fmt.Printf("[%s] deleted!\n", dir)
		}(dir)
	}
	wg.Wait()
}

func (d *UpYunDeployer) handleFile(wg *sync.WaitGroup, filename string, relativeFilename string) {
	wg.Add(1)
	defer wg.Done()

	data, err := ioutil.ReadFile(filename)
	if err != nil {
		fmt.Printf("[%s] read file failed, reason: %s\n", relativeFilename, err)
		return
	}

	remoteFileInfo, err := d.up.GetInfo(relativeFilename)
	putObjectConfig := &upyun.PutObjectConfig{
		Path:      relativeFilename,
		LocalPath: filename,
		Headers: map[string]string{
			"Content-Type": http.DetectContentType(data),
		},
	}

	if upyun.IsNotExist(err) {
		d.uploadFile(putObjectConfig, false)
		return
	}

	if remoteFileInfo.MD5 == fmt.Sprintf("%x", md5.Sum(data)) {
		fmt.Printf("[%s] cached!\n", relativeFilename)
		return
	}

	err = d.up.Delete(&upyun.DeleteObjectConfig{Path: relativeFilename})
	if err != nil {
		return
	}

	d.uploadFile(putObjectConfig, true)
}

func (d *UpYunDeployer) uploadFile(putObjectConfig *upyun.PutObjectConfig, refresh bool) {
	action := "upload"
	if refresh {
		action = "refresh"
	}
	err := d.up.Put(putObjectConfig)
	if err == nil {
		fmt.Printf("[%s] %sed!\n", putObjectConfig.Path, action)
	} else {
		fmt.Printf("[%s] %s failed !\n", putObjectConfig.Path, action)
	}
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
			fmt.Println("err:", err)
		}
	}()

	for obj := range objsChan {
		obj.Name = filepath.Join(path, obj.Name)
		ch <- obj
	}
	ch <- nil
}

func getCurrentExecutePath() string {
	ex, err := os.Executable()
	if err != nil {
		panic(err)
	}
	return filepath.Dir(ex)
}

func getBasicDir(level int) string {
	segments := strings.Split(strings.Trim(getCurrentExecutePath(), "/"), "/")
	max := len(segments) - level

	dir := ""
	for i, segment := range segments {
		if i < max {
			dir = filepath.Join(dir, segment)
		}
	}

	return dir
}

var bucket = flag.String("bucket", "", "")
var operator = flag.String("operator", "", "")
var password = flag.String("password", "", "")

func main() {
	flag.Parse()

	up := upyun.NewUpYun(&upyun.UpYunConfig{
		Bucket:   *bucket,
		Operator: *operator,
		Password: *password,
	})

	deployer := &UpYunDeployer{
		up:       up,
		basicDir: getCurrentExecutePath(),
	}

	begin := time.Now()

	deployer.UploadFiles()

	fmt.Printf("done! consumed: %s\n", time.Since(begin))
}
