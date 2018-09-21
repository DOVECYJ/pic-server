package main

import (
	"bufio"
	"bytes"
	"io/ioutil"
	"log"
	"mime/multipart"
	"net/http"
	"os"
	"strings"
)

//TODO:
//支持文件夹上传
//需要解析c.pictures中的每一项，
//----如果是路径
//--------获得路径下的所有图片文件并添加到slice中
//----将路径删除
//将所有文件合并到c.pictures中

type client struct {
	url      string
	pictures []string
}

//单文件上传
func (c *client) singleUpload() {
	if len(c.pictures) == 0 {
		log.Println("Nothing to upload")
		return
	}
	if _, err := os.Stat(c.pictures[0]); err != nil {
		log.Fatalln("upload file not exist")
		return
	}

	file, err := os.Open(c.pictures[0])
	if err != nil {
		log.Fatalf("can't open %s\n\terr: %s", c.pictures[0], err)
		return
	}
	defer file.Close()

	var buf bytes.Buffer
	mtw := multipart.NewWriter(&buf)
	w, err := mtw.CreateFormFile("image", c.pictures[0])
	if err != nil {
		log.Fatalf("create multipart file error: %s\n", err)
		return
	}
	bufio.NewReader(file).WriteTo(w)
	mtw.Close()

	contentType := "multipart/form-data;boundary=" + mtw.Boundary()
	resp, err := http.Post(c.url, contentType, &buf)
	if err != nil {
		log.Fatalln("[post err]", err)
		return
	}
	defer resp.Body.Close()
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		log.Fatalln("[ready body err]", err)
		return
	}
	log.Println("[result]", string(body))
}

//多文件上传
func (c *client) multiUpload() {
	if len(c.pictures) == 0 {
		log.Println("Nothing to upload")
		return
	}
	var buf bytes.Buffer
	mtw := multipart.NewWriter(&buf)
	for i := range c.pictures {
		if _, err := os.Stat(c.pictures[i]); err != nil {
			log.Fatalf("file %s is not exist\n", c.pictures[i])
			continue
		}

		file, err := os.Open(c.pictures[i])
		if err != nil {
			log.Fatalf("can't open %s\n\terr: %s", c.pictures[i], err)
		}
		defer file.Close()

		w, err := mtw.CreateFormFile("image", c.pictures[i])
		if err != nil {
			log.Fatalf("create multipart file error: %s\n", err)
			continue
		}
		bufio.NewReader(file).WriteTo(w)
	}
	mtw.Close()

	contentType := "multipart/form-data;boundary=" + mtw.Boundary()
	resp, err := http.Post(c.url, contentType, &buf)
	if err != nil {
		log.Fatalln("[post err]", err)
		return
	}
	defer resp.Body.Close()
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		log.Fatalln("[ready body err]", err)
		return
	}
	log.Println("[result]", string(body))
}

//设置上传地址
func (c *client) SetUrl(url string) {
	c.url = url
}

//设置上传的文件
func (c *client) SetUploadFiles(files []string) {
	c.pictures = files
}

//创建上传客户端
func NewClient() *client {
	return &client{
		url:      "http://localhost:8765",
		pictures: []string{},
	}
}

//TODO:
//将解析命令行的功能单独封装出去
//解析命令行参数
func ParseArgs(args []string) map[string][]string {
	m := make(map[string][]string)
	name := "undef"
	for i := 0; i < len(args); i++ {
		if strings.HasPrefix(args[i], "-") {
			name = args[i][1:]
			m[name] = []string{}
		} else {
			m[name] = append(m[name], args[i])
		}
	}
	return m
}

func main() {
	args := os.Args[1:]
	marg := ParseArgs(args)

	c := NewClient()
	if val, ok := marg["url"]; ok && len(val) > 0 {
		url := val[0]
		if !strings.HasPrefix(url, "http") {
			url = "http://" + url //url必须以http://开始
		}
		c.SetUrl(url)
	}
	if val, ok := marg["file"]; ok {
		c.SetUploadFiles(val)
	}
	if len(c.pictures) == 1 {
		c.singleUpload()
		return
	}
	c.multiUpload()
}
