package youtube

import (
	"math"
	"html"
	"os"
	"encoding/xml"
	"strings"
	"io/ioutil"
	"net/http"
	"fmt"
)

type SubtilesDL struct{
	langs [] string
}

type Subtitls struct{
	fileName string
	SubtitlesInfo []SubtitlesInfo
}

type SubtitlesInfo struct{
	BaseUrl string `json:"baseUrl"`
	LanguageCode string `json:"languageCode"`
}

type Transcript struct{
	Text [] SubtitlesContent `xml:"text"`
}

type SubtitlesContent struct{
	Start float64 `xml:"start,attr"`
	Duaring float64 `xml:"dur,attr"`
	Text string `xml:",innerxml"`
}

func NewSubtiteDL(lang ... string) SubtilesDL{
	return SubtilesDL {langs : lang}
}

func (st *SubtilesDL) Download(subtitls Subtitls)  error{
	for _, info := range subtitls.SubtitlesInfo {
		for _, lang := range st.langs {
			if info.LanguageCode == lang {
				st.Do(subtitls.fileName, info)
			}
		}
	}
	return nil
}

func (st * SubtilesDL) Do(fileName string, info SubtitlesInfo) error{
	resp, err := http.Get(info.BaseUrl)
	if err != nil {
		fmt.Printf("%s", err)
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
	//fmt.Println(string(body))
	xml_rep := strings.Replace(string(body), "&amp;", "&", -1)
	v := Transcript{}
	err = xml.Unmarshal([]byte(xml_rep), &v)
	if err != nil{
		fmt.Println(err)
		return err
	}
	filepath := buildFilename(fileName, info)
	out, err := os.Create(filepath)
	if err != nil {
		fmt.Println(err)
		return err
	}
	texts := v.Text
	s := len(texts)
	for i:=0; i<s; i++{
		out.WriteString(fmt.Sprintf("%d\r\n",i+1))
		out.WriteString(fmt.Sprintf("%s --> %s\r\n",secToTime(texts[i].Start), secToTime(texts[i].Start + texts[i].Duaring)))
		out.WriteString(fmt.Sprintf("%s\r\n\r\n",html.UnescapeString(texts[i].Text)))
	}

	out.Close()
	return nil
}

func buildFilename(name string, info SubtitlesInfo) string{
	return name + "." + info.LanguageCode + ".srt"
}

func secToTime(n float64) string{
	sec := math.Mod(n , float64(60))
	min := int(n / 60) % 60
	hour := int(n / 3600)
	
	fmt_sec := fmt.Sprintf("%.3f", sec)
	if(len(fmt_sec) < 6 ){
		fmt_sec = "0" + fmt_sec
	}
	return fmt.Sprintf("%02d:%02d:%s", hour, min, strings.Replace(fmt_sec, "." , ",", 1))
}