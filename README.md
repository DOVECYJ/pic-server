# pic-server
a picture server with golang

## 服务器

```go
func main() {
	server.Run(":8888")
}
```

在浏览器访问`localhost:8888`就可以上传图片了，同时`localhost:8888/concurrent`也可以上传图片。

点击上传以后会显示上传的结果。

访问`localhost:8888/home`可以查看上传的图片。

访问`localhost:8888/del`可以删除上传的图片。

上传图片时会尽量保证用图片原名，重名图片会被重命名。

## 客户端

1. 直接用`go run`命令运行：
```go run client.go -url "localhost:8888" -file F://a.jpg E://b.png```

2. 编译后运行：
```go builg client.exe```
然后在powershell中输入`./client.exe-file F://a.jpg E://b.png`

