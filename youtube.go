package youtube

import (
	"sync"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"
	URL "net/url"
	"regexp"
	"strings"
	. "./zssutils"
)

func NewYoutube(debug bool) *Youtube {
	return &Youtube{DownloadPercent: make(chan int64, 100)}
}

type stream map[string]string

type VideoInfo struct{
	ID 			string
	Filename 	string
	Url 		string
	SubtitlesInfo 	[]SubtitlesInfo
}


type Youtube struct {
	StreamList        []stream
	listID			  string
	VideoInfoList	  [] VideoInfo
	destPath 		  string
	DownloadPercent   chan int64
	contentLength     float64
	totalWrittenBytes float64
	downloadLevel     float64
}


func (y *Youtube) isPlayList(url string) (bool, error) {
	if(strings.Contains(url, "list")){
		Url, _ := URL.Parse(url)
		kv,  _ := URL.ParseQuery(Url.RawQuery) 
		list, ok := kv["list"]
		if ok {
			y.listID = list[0]
			return true, nil
		}else{
			fmt.Printf("[Warn] parameter list not obtained, %s", kv)
		}
	}
	return false, fmt.Errorf("playlist parse error")
}

func (y *Youtube) listVideoUrl() error {
	resp, err := http.Get("https://www.youtube.com/playlist?list=" + y.listID)
	if err != nil {
		fmt.Printf("get list %s", err)
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return err
	}
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	listpage := string(body)
	videoidlist := regexp.MustCompile(`data-video-id="(.+?)"`).FindAllStringSubmatch(listpage, -1)
	y.VideoInfoList = make([]VideoInfo, len(videoidlist))
	for i:=0; i<len(videoidlist);i++ {
		y.VideoInfoList[i].ID = videoidlist[i][1]
	}
	return err
}

func (y *Youtube) StartDownload(url string, destPath string) error {
	y.destPath = destPath 
	yn, _ := y.isPlayList(url)
	if yn {
		if err := y.listVideoUrl(); err != nil{
			return err
		}
	} else {
		videoId, err := y.obtainVideoID(url)
		if err != nil {
			return fmt.Errorf("findVideoID error=%s", err)
		}
		y.VideoInfoList = []VideoInfo{ VideoInfo{ID:videoId} }
	}
	y.download()
	return nil
}

func (y * Youtube) download(){
	coverRes := make(chan Resource, 5)

	videoDL := NewMultitaskDL(5)
	videoDL.StartSyncDownload(coverRes)

	subtitleDL := NewSubtiteDL("zh", "zh-CN", "cn", "en")
	wg := new(sync.WaitGroup)

	vis := make (chan VideoInfo, len(y.VideoInfoList))
	for _,vi := range y.VideoInfoList{
		vis <- vi
	}
	for i:=0; i<3; i++{
		wg.Add(1)
		go func (){
			defer wg.Done()
			done := false
			for !done {
				select{
				case vi := <-vis:
					videoStream, err := y.getVideoinfo(&vi)
					if err != nil {
						fmt.Printf("%s\n", err)
					}else{
						targetStream := videoStream[0]

						fileName := regexp.MustCompile(`[*\\?/|:"]`).ReplaceAllString(targetStream["title"],"")
						fileExt := regexp.MustCompile(`video/(.+?);`).FindStringSubmatch(targetStream["type"])[1]
	
						coverRes <- Resource{
							Url : targetStream["url"] + "&signature=" + targetStream["sig"],
							Fileinfo : FileInfo{
								Name : fileName,
								Dir : y.destPath,
								Ext : fileExt,
							},
						}
						subtitleDL.Download(Subtitls{
							fileName : fileName,
							SubtitlesInfo : vi.SubtitlesInfo,
						})
					}
					
				default:
					done = true
				}
			}
			
		}()
	}
	wg.Wait()
	videoDL.ShutdownAndWait()

}

func (y *Youtube) getVideoinfo(videoinfo *VideoInfo) ([]stream , error) {

	videoRsp, err := y.requestVideoInfo(videoinfo.ID)
	if err != nil {
		return nil, err
	}

	answer, err := URL.ParseQuery(videoRsp)
	if err != nil {
		return nil, err
	}

	status, ok := answer["status"]
	if !ok {
		err = fmt.Errorf("no response status found in the server's answer")
		return nil,err
	}
	if status[0] == "fail" {
		reason, ok := answer["reason"]
		if ok {
			err = fmt.Errorf("'fail' response status found in the server's answer, reason: '%s'", reason[0])
		} else {
			err = errors.New(fmt.Sprint("'fail' response status found in the server's answer, no reason given"))
		}
		return nil,err
	}
	if status[0] != "ok" {
		err = fmt.Errorf("non-success response status found in the server's answer (status: '%s')", status)
		return nil,err
	}

	// read the streams map
	streamMap, ok := answer["url_encoded_fmt_stream_map"]
	if !ok {
		err = errors.New(fmt.Sprint("no stream map found in the server's answer"))
		return nil,err
	}

	// read each stream
	streamsList := strings.Split(streamMap[0], ",")

	var streams []stream
	for _, streamRaw := range streamsList {
		streamQry, err := URL.ParseQuery(streamRaw)
		if err != nil {
			continue
		}
		var sig string
		if _, exist := streamQry["sig"]; exist {
			sig = streamQry["sig"][0]
		}

		streams = append(streams, stream{
			"quality": streamQry["quality"][0],
			"type":    streamQry["type"][0],
			"url":     streamQry["url"][0],
			"sig":     sig,
			"title":   answer["title"][0],
			"author":  answer["author"][0],
		})
		//y.log(fmt.Sprintf("Stream found: quality '%s', format '%s'", streamQry["quality"][0], streamQry["type"][0]))
	}

	pr, ok := answer["player_response"]
	if ok {
		ppr, _ := URL.PathUnescape(pr[0])
		//fmt.Println(ppr)
		str := regexp.MustCompile(`"captionTracks":(\[.+?\])`).FindStringSubmatch(ppr)
		//fmt.Println(str)
		p := &[]SubtitlesInfo{}
		if len(str) > 0 {
			json.Unmarshal([]byte(str[1]), p)
		}
		videoinfo.SubtitlesInfo = *p
		// for _,subTexturl := range *p{
		// 	fmt.Println(subTexturl)
		// }
		//fmt.Println((*p)[0].BaseUrl)
	}
	
	return streams, nil
}

func (y *Youtube) requestVideoInfo(ID string) (string, error) {
	url := "http://youtube.com/get_video_info?video_id=" + ID
	resp, err := http.Get(url)
	if err != nil {
		fmt.Printf("get video info %s", err)
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return "", err
	}
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	//y.videoInfo = string(body)
	return string(body), nil
}

func (y *Youtube) obtainVideoID(url string) (string, error) {
	videoID := url
	if strings.Contains(videoID, "youtu") || strings.ContainsAny(videoID, "\"?&/<%=") {
		reList := []*regexp.Regexp{
			regexp.MustCompile(`(?:v|embed|watch\?v)(?:=|/)([^"&?/=%]{11})`),
			regexp.MustCompile(`(?:=|/)([^"&?/=%]{11})`),
			regexp.MustCompile(`([^"&?/=%]{11})`),
		}
		for _, re := range reList {
			if isMatch := re.MatchString(videoID); isMatch {
				subs := re.FindStringSubmatch(videoID)
				videoID = subs[1]
			}
		}
	}
	if strings.ContainsAny(videoID, "?&/<%=") {
		return "",errors.New("invalid characters in video id")
	}
	if len(videoID) < 10 {
		return "",errors.New("the video id must be at least 10 characters long")
	}
	return videoID, nil
}