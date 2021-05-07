/**
*
*	parallel download files
* 	downloading with progress info and  retry until completed
*	zssty2010@gmail.com
*/
package zssutils

import (
	"sync/atomic"
	"sync"
	"time"
	"io"
	"fmt"
	"strconv"
	"net/http"
	"os"
	"path/filepath"
)

func test(){
	downloader := NewMultitaskDL(5)
	res := []Resource{
			Resource{
				Url : "https://stackoverflow.com/questions/37780520/unknown-field-in-struct-literal",
				Fileinfo : FileInfo{Name:"a", Dir:"a:\\",Ext:"",},
			},
			Resource{
				Url : "https://www.youtube.com/playlist?list=PLQVvvaa0QuDfKTOs3Keq_kaG2P55YRn5v",
				Fileinfo : FileInfo{Name:"bbbb", Dir:"a:\\",Ext:"",},
			},
			Resource{
				Url : "http://audio.xmcdn.com/group8/M01/BD/21/wKgDYFZgcySidwskAKKvuoFtqdE653.m4a",
				Fileinfo : FileInfo{Name:"cccccaaa", Dir:"a:\\",Ext:"",},
			},
		}
	downloader.StartDownload(res)
}


type MultitaskDL struct{
	ThreadNum 		int
	shutdown 		bool
	progressChan 	chan TaskProgress
	tracer			Tracer
	wg				sync.WaitGroup
}

func NewMultitaskDL(n int) MultitaskDL{
	return MultitaskDL{
		ThreadNum : n,
		shutdown : false,
	}
}

//define download from where and where to save
type Resource struct{
	Url 		string
	Fileinfo 	FileInfo
}
type FileInfo struct{
	Name 		string
	Dir 		string
	Ext 		string
}


type statistics struct{
	total 			int32
	proccessed  	int32
	completed 		int32
	progressInfo	map[string]TaskProgress
}

func (stat * statistics) add(tp TaskProgress){
	if tp.state != Processing {
		stat.proccessed += 1
		delete(stat.progressInfo, tp.name)
		
		if tp.state == Completed {
			stat.completed ++
		}
	} else {
		stat.progressInfo[tp.name] = tp
	}
}

type Tracer struct{
	stat 		*statistics
	print		func(TaskProgress)
}

func (tracer * Tracer) trace(t TaskProgress) {
	if tracer.print == nil{
		tracer.print = NewPrinter(tracer.stat)
	}
	tracer.stat.add(t)
	tracer.print(t)
}

func (tracer * Tracer) isAllDone() bool{
	return tracer.stat.proccessed == tracer.stat.total
}

const(
	Processing = iota
	Completed 
	Failed
)

//inner struct to trace download progress
type TaskProgress struct{
	name 			string
	contentLength 	int
	loadedLength	int
	delta			int
	startTime		int64
	state			int
	err				error
	taskTrace chan TaskProgress
}

func (progress * TaskProgress) Write(p [] byte) (n int, err error){
	size := len(p)
	progress.inc(size)
	return size, nil
}

func (progress *TaskProgress) inc(len int){
	progress.delta += len
	progress.state = Processing
	progress.loadedLength += len
	progress.notify()
}

func (progress *TaskProgress) Finish() error{
	progress.state = Completed
	progress.notify()
	return nil
}

func (progress *TaskProgress) Failed(err error) error{
	progress.err = err
	progress.state = Failed
	progress.notify()
	return err
}

func (progress *TaskProgress) notify(){
	if progress.taskTrace != nil {
		progress.taskTrace <- *progress
	}
}


//golang not support default value. so we can use closures to do like this
func NewPrinter(stat *statistics) (func (TaskProgress)) {
	progressChar := []string{"-","\\","/",}
	progressIdx := 0
	lastnamelength :=0

	KB:= int64(1024)
	MB:= int64(KB * 1024)
	getUnit := func (number int64)string{ //字节单位
		if number >= MB {
			return fmt.Sprintf("%dMB", number/MB)
		}else if number >=KB {
			return fmt.Sprintf("%dKB", number/KB)
		}
		return fmt.Sprintf("%dB", number)
	}

	caclSpeed := func (t TaskProgress) string{//计算平均速度
		timedev := (time.Now().UnixNano() - t.startTime) / 1000000000
		if timedev > 0 {
			return fmt.Sprintf("%s", getUnit(int64(t.delta) / timedev))
		}else {
			return "-"
		}
	}

	print := func (){
		progress := progressChar[progressIdx]
		for _, t := range stat.progressInfo {
			if t.contentLength > 0 {
				progress = fmt.Sprintf("%d",t.loadedLength * 100 / t.contentLength)
			}
			devLength := lastnamelength - len(t.name) + 1
			lastnamelength = len(t.name)

			//方式一、用\t来覆盖后面多出来的上一文件名的字符，很容易导致跨行
			//fmt.Printf("%d/%d-%d (%s%%) %s\t\t\t\r", stat.completed, stat.total, stat.proccessed - stat.completed, progress, t.name)
			
			//方式二、用循环打印空格来覆盖多出来的字符
			// fmt.Printf("%d/%d-%d (%s%%) %s", stat.completed, stat.total, stat.proccessed - stat.completed, progress, t.name)
			// for i:= devLength; i >0; i--{
			// 	fmt.Printf("%s", " ")
			// }
			// fmt.Printf("\r")

			//方式三、使用%+s打印空格
			if devLength <= 0 {
				devLength = 0
			}
			speed := caclSpeed(t)
			fmt.Printf("%d/%d %d (%s%%) - %s/s %s%+" + strconv.Itoa(devLength) + "s\r", stat.proccessed, stat.total, stat.proccessed - stat.completed, progress, speed, t.name, " ")
			if progressIdx += 1; progressIdx >= 3 {
				progressIdx = 0
			}
			break
		}
	}

	go func(){
		ticker := time.NewTicker(time.Second*1)
		for _ = range ticker.C {
			print()
		}
	}()
	
	return func (t TaskProgress){
		if t.state!= Processing {
			if t.state == Completed {
				fmt.Printf("%s\t\t\t\t\t\n", t.name)
			} else if t.state == Failed {
				fmt.Printf("%s\t%s\t\t\t\t\n", t.name, t.err)
			}
			print()
		}
	}
}

//main method start to download files
func (dl *MultitaskDL) StartDownload(resources [] Resource){
	size := len(resources)
	//wg := new(sync.WaitGroup)
	data := make(chan Resource, size)
	progressChan := make(chan TaskProgress, size) //trace pipe print progress and detect finish event
	stat := statistics{total : int32(size), completed : 0, progressInfo : make(map[string]TaskProgress),} //just human readable
	tracer := Tracer{stat : &stat}
	for _, res  := range resources{	//push all tasks into pipe
		data <- res
	}

	if dl.ThreadNum <= 0 {//necessary check
		dl.ThreadNum = 1
	}
	
    for i := 0; i < dl.ThreadNum; i++ {
        //wg.Add(1)
        go func() {
			//defer wg.Done()
			done := false
			for !done {
				select {
					case res := <-data:
						if err := dl.Do(res, progressChan); err != nil {
						}
					default:
						done = true
				}
			}
        }()
	}
	
	for t := range progressChan{
		tracer.trace(t)
		if tracer.isAllDone() {
			break
		}
	}
	fmt.Printf("\ntotal:%d got:%d failed:%d", stat.total, stat.completed, stat.proccessed - stat.completed)
			
	//wg.Wait()
}


func (dl *MultitaskDL) StartSyncDownload(resourceChan chan Resource){
	dl.progressChan = make(chan TaskProgress, dl.ThreadNum) //trace pipe print progress and detect finish event
	stat := statistics{total : 0, completed : 0, progressInfo : make(map[string]TaskProgress),} //just human readable
	dl.tracer = Tracer{stat : &stat}

	if dl.ThreadNum <= 0 {//necessary check
		dl.ThreadNum = 1
	}
	
    for i := 0; i < dl.ThreadNum; i++ {
		dl.wg.Add(1)
        go func() {
			defer dl.wg.Done()
			done := false
			for !done {
				select {
					case res := <- resourceChan:
						atomic.AddInt32(&stat.total, 1)
						if err := dl.Do(res, dl.progressChan); err != nil {
						}
					default:
						if dl.shutdown { //if outer system invoked shutdown. thread will exit when no data in chan
							done = true
						}
				}
			}
        }()
	}
	
	
	go func (){
		
		for t := range dl.progressChan{
			dl.tracer.trace(t)
		}
		fmt.Printf("\ntotal:%d got:%d failed:%d", dl.tracer.stat.proccessed, dl.tracer.stat.completed, dl.tracer.stat.proccessed - dl.tracer.stat.completed)
	}()
}

func (dl *MultitaskDL) ShutdownAndWait(){
	dl.shutdown = true
	dl.wg.Wait()
	close(dl.progressChan)
}

func (dl *MultitaskDL) Do(res Resource, taskTrace chan TaskProgress) error{
	destFile := buildfilepath(res.Fileinfo)

	taskProgress := &TaskProgress{ //bind trace and trace chan. then we can use trace to send notice
		name : res.Fileinfo.Name,
		state : Processing,
		startTime : time.Now().UnixNano(),
		taskTrace : taskTrace,
	}
	taskProgress.notify()

	if _, err := os.Stat(destFile); err == nil {//完成了
		return taskProgress.Finish()
	}

	end := 0 //request content-range end
	var retries = 1
	for times := 0 ; times < retries; times++{ //retry for three times
		if rsp, err := http.Head(res.Url); err == nil {
			maps := rsp.Header
			cl := maps["Content-Length"]
			
			if len(cl) > 0 {
				end, _ = strconv.Atoi(cl[0])
				taskProgress.contentLength = end
			}
			break
		} else {
			if times >= retries -1 {
				return taskProgress.Failed(err)
			}
		}
	}
	
	tmpFilepath := destFile + ".dl"

	if err := os.MkdirAll(filepath.Dir(destFile), 666); err != nil {
		return taskProgress.Failed(err)
	}
	outstream, err := os.OpenFile(tmpFilepath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, os.ModeTemporary)
	if err != nil {
		return taskProgress.Failed(err)
	}

	mw := io.MultiWriter(outstream, taskProgress)
	for true {
		begin := 0	//get content-range begin from temp file
		if stat, err := os.Stat(tmpFilepath); err == nil {
			begin = int(stat.Size())
			taskProgress.loadedLength = begin
		}

		if	end > 0 && begin == end {
			return dl.finishDownload(tmpFilepath, destFile, taskProgress)
		}else if( begin > end){
			fmt.Printf("file may have been damaged %s", tmpFilepath)
		}

		resp, err := HttpGet(res.Url, begin, end)
		if err != nil || (resp.StatusCode != 206 && resp.StatusCode != 200) {
			time.Sleep(1500)
			continue
		}
		
		_, err = io.Copy(mw, resp.Body); 
		resp.Body.Close() //try to close http reader. 
		if err == nil {
			break
		}
	}

	outstream.Close()	//close file writer
	return dl.finishDownload(tmpFilepath, destFile, taskProgress)
}

func (dl * MultitaskDL) finishDownload(tempFile, destFile string, taskProgress *TaskProgress) error{
	if err := os.Rename(tempFile, destFile); err != nil{
		return taskProgress.Failed(err)
	}
	return taskProgress.Finish()
}


func HttpGet(url string, start int, end int)(*http.Response, error){
	client := &http.Client {
		//Timeout:-1,
	}
	req, _ := http.NewRequest("GET", url, nil)  
	range_header := fmt.Sprintf("bytes=%d-%d", start, end) // Add the data for the Range header 
	req.Header.Add("Range", range_header)
	return client.Do(req)
}

func buildfilepath (fi FileInfo) string{
	fp := filepath.Clean(fi.Dir) + string(filepath.Separator) + fi.Name
	if fi.Ext != "" {
		return fp + "." + fi.Ext
	}
	return fp
}