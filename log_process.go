package main

import (
	"fmt"
	"strings"
	"time"
)

type LogProcess struct {
	rc          chan string // read channel
	wc          chan string // write channel
	influxDBDsn string      // influx data source
}

type ReadFromFile struct {
	path        string      // 读取文件路径
}

// 读取文件
func (l *LogProcess) ReadFromFile() {
	line := "message"
	l.rc <- line

}

// 解析文件
func (l *LogProcess) Process() {
	data := <-l.rc
	l.wc <- strings.ToUpper(data)
}

// 写入文件
func (l *LogProcess) WriteToInfluxDB() {
	fmt.Print(<-l.wc)

}

func main() {
	lp := &LogProcess{
		rc:          make(chan string),
		wc:          make(chan string),
		path:        "tmp/access.log",
		influxDBDsn: "username&password",
	}

	go lp.ReadFromFile()
	go lp.Process()
	go lp.WriteToInfluxDB()

	time.Sleep(1*time.Second)


}
