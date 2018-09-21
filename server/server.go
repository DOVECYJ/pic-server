package server

import (
	"crypto/md5"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/satori/go.uuid"
)

//TODO:
//支持自定义图片保存路径

const (
	maxImgSize     = 0xA << 20
	maxUploadCount = 1 << 10
	imgPath        = "./images/"
	urlfmt         = "localhost%s/img/"
)

var imgUrl = "localhost:8765/img/"

var exts = []string{"JPG", "JPEG", "PNG", "BMP", "GIF", "EPS", "TGA", "TIFF", "PSD"}

func CheckExt(ext string) bool {
	if ext[0] == '.' {
		ext = ext[1:]
	}
	ext = strings.ToUpper(ext)
	for _, v := range exts {
		if ext == v {
			return true
		}
	}
	return false
}

//处理多文件上传的Get请求
func multi_uploadHandler_get(c *gin.Context) {
	c.HTML(200, "upload.html", nil)
}

//处理并发文件上传的Get请求
func go_uploadHandler_get(c *gin.Context) {
	c.HTML(200, "upload_concurrent.html", nil)
}

//处理多文件上传的Post请求
func multi_uploadHandler_post(c *gin.Context) {
	if err := c.Request.ParseMultipartForm(maxUploadCount); err != nil {
		//解析表单出错，上传失败
		c.String(400, "<h2>解析表单出错</h2>")
		return
	}
	files := c.Request.MultipartForm.File["image"]
	var errs []error
	var failed, urls []string
	for i := range files {
		if files[i].Size > maxImgSize { //文件过大，上传失败
			failed = append(failed, files[i].Filename)
			errs = append(errs, errors.New("Picture is bigger than 10M T_T"))
			continue
		}
		file, err := files[i].Open()
		if err != nil { //文件打开失败，上传失败
			failed = append(failed, files[i].Filename)
			errs = append(errs, err)
			continue
		}
		defer file.Close()
		url, err := saveFile(files[i].Filename, file)
		if err != nil { //保存失败， 上传失败
			failed = append(failed, files[i].Filename)
			errs = append(errs, err)
			continue
		}
		//上传成功
		urls = append(urls, url)
	}
	//显示上传结果
	httpcode, code := 200, 0
	if len(errs) == 0 {
		code = 0 //全部上传成功
	} else if len(errs) < len(files) {
		code = 1 //部分成功
	} else {
		code = 2 //全部失败
		httpcode = 500
	}
	c.HTML(httpcode, "result.html", gin.H{
		"code":   code,
		"failed": failed,
		"urls":   urls,
		"errs":   errs,
	})
}

//TODO:
//将闭包拆分出来
//处理并发文件上传的Post请求
func go_uploadHandler_post(c *gin.Context) {
	fmt.Println("[request]", *c.Request)
	if err := c.Request.ParseMultipartForm(maxUploadCount); err != nil {
		//解析表单出错，上传失败
		c.String(400, "<h2>解析表单出错</h2>")
		return
	}
	files := c.Request.MultipartForm.File["image"]
	var errs []error
	var failed, urls []string

	var urlchan = make(chan string)
	var failchan = make(chan string)
	var errchan = make(chan error)
	var saveChan = make(chan *multipart.FileHeader, 10)

	go func(files []*multipart.FileHeader) { // 将文件发送到通道
		for i := range files {
			if files[i].Size > maxImgSize { //文件过大，上传失败
				failchan <- files[i].Filename
				errchan <- errors.New("Picture is bigger than 10M T_T")
				continue
			}
			saveChan <- files[i]
		}
		close(saveChan)
	}(files)

	go func() { //保存文件
		var lock sync.Mutex
		for elem := range saveChan {
			go func(fh *multipart.FileHeader) {
				file, err := fh.Open()
				if err != nil {
					//文件打开失败，上传失败
					failchan <- fh.Filename
					errchan <- err
					return
				}
				defer file.Close()

				filename := filepath.Base(fh.Filename)
				lock.Lock()
				if _, err := os.Stat(imgPath + filename); err != nil {
					out, err := os.Create(imgPath + filename)
					lock.Unlock()
					if err != nil {
						failchan <- fh.Filename
						errchan <- err
						return
					}

					_, err = io.Copy(out, file)
					if err != nil {
						failchan <- fh.Filename
						errchan <- err
						out.Close()
						lock.Lock()
						err = os.Remove(imgPath + filename)
						lock.Unlock()
						if err != nil {
							log.Fatalf("can't delete %s\n", filename)
						}
						return
					}
					out.Close()
					urlchan <- (imgUrl + filename)
				} else {
					lock.Unlock()
					ext := filepath.Ext(filename)
					id, err := uuid.NewV4()
					if err == nil {
						filename = id.String() + ext
					} else {
						md := md5.New()
						md.Write([]byte(filename))
						sum := md.Sum([]byte(time.Now().String()))
						filename = hex.EncodeToString(sum) + ext
					}
					path := imgPath + filename
					out, err := os.Create(path)
					if err != nil {
						failchan <- fh.Filename
						errchan <- err
						return
					}
					//defer out.Close()
					_, err = io.Copy(out, file)
					if err != nil {
						failchan <- fh.Filename
						errchan <- err
						out.Close()
						lock.Lock()
						err = os.Remove(path)
						lock.Unlock()
						if err != nil {
							log.Fatalf("can't delete %s\n", filename)
						}
						return
					}
					out.Close()
					urlchan <- (imgUrl + filename)
				}
			}(elem)
		}
	}()

	//等待保存结果
	for i := 0; i < len(files); i++ {
		select {
		case url := <-urlchan:
			urls = append(urls, url)
		case err := <-errchan:
			errs = append(errs, err)
			failed = append(failed, <-failchan)
		}
	}

	//上传结果
	httpcode, code := 200, 0
	if len(errs) == 0 {
		code = 0 //全部上传成功
	} else if len(errs) < len(files) {
		code = 1 //部分成功
	} else {
		code = 2 //全部失败
		httpcode = 500
	}
	c.HTML(httpcode, "result.html", gin.H{
		"code":   code,
		"failed": failed,
		"urls":   urls,
		"errs":   errs,
	})
}

//处理查看图片请求
func lookHandler(c *gin.Context) {
	imgs := allPicture(imgPath)
	fmt.Println(imgs)
	for i := range imgs {
		imgs[i] = strings.Replace(imgs[i], "images\\", "/picture/", 1)
	}
	c.HTML(200, "index.html", gin.H{"images": imgs})
}

//处理删除图片的Get请求
func delHandler_get(c *gin.Context) {
	imgs := allPicture(imgPath)
	m := make(map[string]string)
	for i := range imgs {
		m[imgs[i]] = strings.Replace(imgs[i], "images\\", "/picture/", 1)
	}
	c.HTML(200, "delete.html", gin.H{"images": m})
}

//处理删除图片的Post请求
func delHandler_post(c *gin.Context) {
	name := c.Request.PostFormValue("path")
	log.Println("[delete name]", name)
	if err := os.Remove(name); err != nil {
		log.Fatalf("can't delete %s\n", name)
	}
	c.Redirect(302, "/del")
}

/*-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-*/

//获得path下所有图片
func allPicture(path string) []string {
	files, err := fileFilter(path, "*", "png|jpg|jpeg|bmp|gif|GIF|PNG")
	if err != nil {
		log.Println("[allPicture_err]", err)
	}
	return files
}

//文件过滤，获取特定类型的文件
func fileFilter(path, patten, extention string) ([]string, error) {
	if _, err := os.Stat(path); err != nil {
		return nil, err
	}
	if patten == "" {
		patten = "*"
	}
	if extention == "" {
		extention = "*"
	}
	pattens := strings.Split(patten, "|")
	extens := strings.Split(extention, "|")
	var result []string
	for i := range pattens {
		for j := range extens {
			filter := path + pattens[i] + "." + extens[j]
			if fs, err := filepath.Glob(filter); err == nil {
				if len(fs) > 0 {
					result = append(result, fs...)
				}
			} else {
				return nil, err
			}
		}
	}
	return result, nil
}

//TODO:
//将文件名判重拆分出来
//保存文件到本地
func saveFile(filename string, src multipart.File) (string, error) {
	filename = filepath.Base(filename)
	if _, err := os.Stat(imgPath + filename); err == nil {
		ext := filepath.Ext(filename)
		id, err := uuid.NewV4()
		if err == nil {
			filename = id.String() + ext
		} else {
			md := md5.New()
			md.Write([]byte(filename))
			sum := md.Sum([]byte(time.Now().String()))
			filename = hex.EncodeToString(sum) + ext
		}
	}
	path := imgPath + filename
	out, err := os.Create(path)
	if err != nil {
		fmt.Println("[create_file_err]", err)
		return "", err
	}
	_, err = io.Copy(out, src)
	if err != nil {
		out.Close()
		if err = os.Remove(path); err != nil {
			log.Fatalf("can't delete %s\n", filename)
		}
		fmt.Println("[copy_file_err]", err)
		return "", err
	}
	out.Close()
	return imgUrl + filename, nil
}

//检查目录是否存在
func checkDir(dir string) bool {
	if _, err := os.Stat(dir); err != nil { //path not exist
		return false
	}
	return true
}

//检查目录是否存在，不存在则创建
func checkAndCreate(dir string) error {
	if !checkDir(dir) {
		return os.MkdirAll(dir, os.ModePerm) //os.ModePerm=0777
	}
	return nil
}

/*-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-*/

//服务器
type Server struct {
	Port string
}

func (s *Server) Run() {
	router := gin.Default()
	router.StaticFS("/picture", http.Dir(imgPath))
	router.LoadHTMLGlob("html/*")

	router.GET("/", multi_uploadHandler_get)
	router.POST("/", multi_uploadHandler_post)
	router.GET("/concurrent", go_uploadHandler_get)
	router.POST("/concurrent", go_uploadHandler_post)
	router.GET("/home", lookHandler)
	router.GET("/del", delHandler_get)
	router.POST("/del", delHandler_post)

	if s.Port != "" {
		imgUrl = fmt.Sprintf(urlfmt, s.Port)
	} else {
		s.Port = ":8765"
	}

	if err := router.Run(s.Port); err != nil {
		log.Fatalf("[server.run error] %s\n", err)
	}
}

func (s *Server) SetPort(port string) {
	s.Port = port
}

func New() *Server {
	return &Server{":8765"}
}

func init() {
	gin.SetMode(gin.ReleaseMode)
	checkAndCreate(imgPath)
}

var defaultServer Server

func SetPort(port string) {
	defaultServer.SetPort(port)
}

func Run(port ...string) {
	if len(port) > 0 {
		defaultServer.SetPort(port[0])
	}
	defaultServer.Run()
}

/*-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-*/
