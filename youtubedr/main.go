package main

import (
	"fmt"
	"os"
	"path/filepath"

	. ".."
)

func main() {
	fmt.Printf("you can find update from https://github.com/zssty2010/youtube-downloader\n")
	currentDir, _ := filepath.Abs(filepath.Dir(os.Args[0]))
	y := NewYoutube(true)
	y.StartDownload(os.Args[1], currentDir)
}
