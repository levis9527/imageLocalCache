package main

import (
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"path"
	"strconv"
	"strings"
	"time"

	registry "golang.org/x/sys/windows/registry"
)

// 全局task列表
var TaskMap = make(map[string]CacheTask)

type CacheTask struct {
	TaskName string     `json:"taskName"`
	TaskId   string     `json:"taskId"`
	TaskItem []TaskItem `json:"taskItem"`
}

type TaskItem struct {
	FileId    string `json:"fileId"`
	FileUrl   string `json:"fileUrl"`
	IsSuccess bool   `json:"isSuccess"`
}

type CacheHandler struct {
}

const DATA_DIR string = "data"
const PORT string = "9876"

var loger *log.Logger

func SayHello(w http.ResponseWriter, r *http.Request) {
	loger.Printf("HandleFunc")
	w.Write([]byte(string("HandleFunc")))
}

func (th *CacheHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {

	// fileUrl := r.URL.RequestURI()
	fileUrl := r.RequestURI

	loger.Println(fileUrl)

	fileSavePath := GetFilePath(fileUrl)

	if IsExist(fileSavePath) {
		loger.Println("文件存在，直接返回")
		// 如果存在，直接返回
		content, err := ioutil.ReadFile(fileSavePath)
		if err != nil {
			loger.Println("Read error")
		}
		w.Header().Set("Cache-Controller", "max-age=36000")
		w.Header().Set("expires", "Fri, 12 Aug 2099 08:43:17 GMT")
		w.Header().Set("etag", fileSavePath)
		w.Write(content)
		return
	}

	loger.Println("文件不存在")
	// 如果文件不存在，下载文件
	resp, err := http.Get(fileUrl)
	if err != nil {
		loger.Println("网络获取图片失败", err)
		panic(err)
	}
	f, err := os.Create(fileSavePath)
	if err != nil {
		loger.Println("创建图片失败", err)
		panic(err)
	}
	_, err = io.Copy(f, resp.Body)
	if err != nil {
		loger.Println("文件拷贝失败", err)
	}

	content, err := ioutil.ReadFile(fileSavePath)
	if err != nil {
		loger.Println("Read error")
	}
	w.Header().Set("Cache-Controller", "max-age=36000")
	w.Header().Set("expires", "Fri, 12 Aug 2099 08:43:17 GMT")
	w.Header().Set("etag", fileSavePath)
	w.Write(content)
	return
}

// func Download(w http.ResponseWriter, r *http.Request) {
// 	content, err := ioutil.ReadFile("./1.jpg")
// 	if err != nil {
// 		log.Println("Read error")
// 	}
// 	w.Header().Set("Cache-Controller", "max-age=36000")
// 	w.Header().Set("expires", "Fri, 12 Aug 2022 08:43:17 GMT")
// 	w.Header().Set("etag", "1.jpg")
// 	w.Write(content)
// }

func GetPac(w http.ResponseWriter, r *http.Request) {
	loger.Println("get Pac File")
	content, err := ioutil.ReadFile("./pac")
	if err != nil {
		loger.Println("Read error")
	}
	w.Header().Set("Content-Type", "application/x-ns-proxy-autoconfig")
	w.Write(content)
}

func GetTaskStatus(w http.ResponseWriter, r *http.Request) {
	taskId := r.URL.Query().Get("taskId")

	if taskId == "" {
		// w.Header().Set("Content-Type", "application/json;charset=UTF-8")
		w.Write([]byte(string("-1")))
		return
	}
	status := getTaskStatus(taskId)
	w.Write([]byte(string(strconv.Itoa(status))))
}

func CreateCacheTask(w http.ResponseWriter, r *http.Request) {
	method := r.Method
	if strings.EqualFold(method, http.MethodOptions) {
		w.WriteHeader(200)
		return
	}
	body, err := ioutil.ReadAll(r.Body)
	if err != nil {
		loger.Printf("read body err, %v\n", err)
		return
	}

	var cacheTask CacheTask
	if err = json.Unmarshal(body, &cacheTask); err != nil {
		loger.Printf("Unmarshal err, %v\n", err)
		return
	}

	createCacheTask(cacheTask)
	w.Write([]byte(string("ok")))
	return
}

// 2. 查询任务状态，传入任务id查询任务状态
func getTaskStatus(taskId string) int {
	// 任务找不到返回-1，否则返回完成度（0-100）
	task, ok := TaskMap[taskId]

	// 如果task列表里面有值，那么直接跳过，表示重新执行task，可以返回task正在执行中
	// 如果task列表没有值，那么插入task列表并开启线程处理文件下载
	if !ok {
		return -1
	} else {
		count := len(task.TaskItem)
		successCount := 0
		for _, item := range task.TaskItem {
			if item.IsSuccess {
				successCount++
			}
		}
		return (successCount / count) * 100
	}
}

// 1. 创建任务
func createCacheTask(task CacheTask) {
	taskId := task.TaskId
	_, ok := TaskMap[taskId]

	// 如果task列表里面有值，那么直接跳过，表示重新执行task，可以返回task正在执行中
	// 如果task列表没有值，那么插入task列表并开启线程处理文件下载
	if ok {
		loger.Println("跳过执行")
	} else {
		TaskMap[taskId] = task
		// 启动线程执行任务
		go execTask(&task)
	}
}

func execTask(task *CacheTask) {
	taskItem := task.TaskItem

	for idx := range taskItem {
		loger.Println("开始调用下载方法下载文件")
		loger.Println("文件url：" + taskItem[idx].FileUrl)
		start := time.Now().Unix()
		downloadFile(taskItem[idx])
		loger.Println("下载完成，用时 ", time.Now().Unix()-start, " 秒")
		// 修改任务item状态为已完成（已完成标记只作为前端展示，不作为后端业务标记）
		taskItem[idx].IsSuccess = true
	}
}

func downloadFile(item TaskItem) {
	// 判断文件是否存在，存在跳过，不存在就通过网络获取并且下载
	fileUrl := item.FileUrl
	// fileName := Md5Sum(fileUrl)
	// ext := path.Ext(fileUrl)
	filePath := GetFilePath(fileUrl)

	if !IsExist(filePath) {
		// 如果文件不存在，下载文件
		resp, err := http.Get(fileUrl)
		if err != nil {
			loger.Println("网络获取图片失败", err)
			panic(err)
		}
		f, err := os.Create(filePath)
		if err != nil {
			loger.Println("创建图片失败", err)
			panic(err)
		}
		_, err = io.Copy(f, resp.Body)
		if err != nil {
			loger.Println("文件拷贝失败", err)
		}
	} else {
		loger.Println("文件存在，不需要下载")
	}
}

// 工具方法
func Md5Sum(str string) string {
	h := md5.New()
	h.Write([]byte(str))
	return hex.EncodeToString(h.Sum(nil))
}

func IsExist(path string) bool {
	_, err := os.Stat(path)
	return err == nil || os.IsExist(err)
}

func GetFilePath(fileUrl string) string {
	fileName := Md5Sum(fileUrl)

	index := strings.LastIndex(fileUrl, "?")
	if index > 0 {
		fileUrl = fileUrl[:index]
	}
	ext := path.Ext(fileUrl)

	return DATA_DIR + "/" + fileName + ext
}

func createRegistry() bool {
	unix := time.Now().Unix()
	regKey := "AutoConfigURL"
	regValue := "http://127.0.0.1:" + PORT + "/pac?t=" + strconv.FormatInt(unix, 10)
	path := "SOFTWARE\\Microsoft\\Windows\\CurrentVersion\\Internet Settings"

	key, exists, err := registry.CreateKey(registry.CURRENT_USER, path, registry.ALL_ACCESS)
	if err != nil {
		loger.Println(err)
	}
	defer key.Close()

	if exists {
		loger.Println("键已存在")
	} else {
		loger.Println("新建注册表键")
	}
	err2 := key.SetStringValue(regKey, regValue)
	if err2 != nil {
		loger.Println(err2)
		return false
	}
	return true
}

func init() {
	// 创建文件夹存放文件
	os.Mkdir(DATA_DIR, os.ModePerm)
	// 创建日志文件夹
	os.Mkdir("log", os.ModePerm)

	file := "./log/" + time.Now().Format("2006-01-02") + ".txt"
	logFile, err := os.OpenFile(file, os.O_RDWR|os.O_CREATE|os.O_APPEND, 0766)
	if err != nil {
		panic(err)
	}
	out := os.Stdout
	loger = log.New(io.MultiWriter(out, logFile), "[imgCacheProxy]", log.LstdFlags|log.Lshortfile) // 将文件设置为loger作为输出
	return
}

func main() {
	// 创建文件夹存放文件
	os.Mkdir(DATA_DIR, os.ModePerm)
	success := createRegistry()
	if !success {
		loger.Println("修改注册表失败")
		println("修改注册表失败，将在30秒后自动退出，也可自行关闭")
		// time.Sleep(30 * time.Second)
		// return
	}
	loger.Println("修改注册表成功，下一步开始")

	http.Handle("/", &CacheHandler{})
	http.HandleFunc("/pac", GetPac)
	http.HandleFunc("/taskStatus", GetTaskStatus)
	http.HandleFunc("/createCacheTask", CreateCacheTask)
	println("开始启动监听======")
	http.ListenAndServe("127.0.0.1:"+PORT, nil)
}
