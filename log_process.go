package main

import (
	"bufio"
	"flag"
	"fmt"
	"github.com/influxdata/influxdb/client/v2"
	"io"
	"log"
	"net/url"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"
)

type Reader interface {
	Read(rc chan []byte)
}

type Writer interface {
	Write(wc chan *Message)
}

type LogProcess struct {
	rc chan []byte
	wc chan *Message

	read  Reader
	write Writer
}

type ReadFromFile struct {
	path string
}

type WriteToInfluxDB struct {
	influxDBDsn string
}

type Message struct {
	TimeLocal                    time.Time
	BytesSent                    int
	Path, Method, Scheme, Status string
	UpstreamTime, RequestTime    float64
}

// 系统状态监控
type SystemInfo struct {
	HandleLine   int     `json:"handleLine"`   // 总处理日志行数
	Tps          float64 `json:"tps"`          // 系统吞出量
	ReadChanLen  int     `json:"readChanLen"`  // read channel 长度
	WriteChanLen int     `json:"writeChanLen"` // write channel 长度
	RunTime      string  `json:"runTime"`      // 运行总时间
	ErrNum       int     `json:"errNum"`       // 错误数
}

func (r *ReadFromFile) Read(rc chan []byte) {
	// 读取模块
	// 打开文件
	f, err := os.Open(r.path)
	if err != nil {
		panic(fmt.Sprint("Open file error:%s", err.Error()))
	}

	/**
	从文件末尾开始逐行读取
	func (f *File) Seek(offset int64, whence int)
	2 means relative to the end
	 */
	f.Seek(0, 2)
	rd := bufio.NewReader(f)

	for {
		line, err := rd.ReadBytes('\n')
		if err == io.EOF {
			time.Sleep(500 * time.Millisecond)
			continue
		} else if err != nil {
			panic(fmt.Sprint("Read file error:%s", err.Error()))
		}
		// 去掉换行符
		rc <- line[:len(line)-1]
	}

}

func (l *LogProcess) Process() {
	// 解析模块
	r := regexp.MustCompile(`([\d\.]+)\s+([^ \[]+)\s+([^ \[]+)\s+\[([^\]]+)\]\s+([a-z]+)\s+\"([^"]+)\"\s+(\d{3})\s+(\d+)\s+\"([^"]+)\"\s+\"(.*?)\"\s+\"([\d\.-]+)\"\s+([\d\.-]+)\s+([\d\.-]+)`)
	loc, _ := time.LoadLocation("Asia/Shanghai")

	// 循环读取channel rc
	for v := range l.rc {
		ret := r.FindStringSubmatch(string(v))
		// 正则匹配失败
		if len(ret) != 14 {
			log.Println("FindStringSubmatch fail:", string(v))
			continue
		}

		message := &Message{}
		t, err := time.ParseInLocation("02/Jan/2006:15:04:05 +0000", ret[4], loc)
		if err != nil {

			log.Println("ParseInLocation fail:", err.Error(), ret[4])
			continue
		}
		message.TimeLocal = t

		byteSent, _ := strconv.Atoi(ret[8])
		message.BytesSent = byteSent

		// GET /foo?query=t HTTP/1.0
		reqSli := strings.Split(ret[6], " ")
		if len(reqSli) != 3 {

			log.Println("strings.Split fail", ret[6])
			continue
		}
		message.Method = reqSli[0]

		u, err := url.Parse(reqSli[1])
		if err != nil {
			log.Println("url parse fail:", err)

			continue
		}
		message.Path = u.Path

		message.Scheme = ret[5]
		message.Status = ret[7]

		upstreamTime, _ := strconv.ParseFloat(ret[12], 64)
		requestTime, _ := strconv.ParseFloat(ret[13], 64)
		message.UpstreamTime = upstreamTime
		message.RequestTime = requestTime

		l.wc <- message
	}
}

func (w *WriteToInfluxDB) Write(wc chan *Message) {
	// 写入模块

	infSli := strings.Split(w.influxDBDsn,"@")

	// Create a new HTTPClient
	c, err := client.NewHTTPClient(client.HTTPConfig{
		Addr:     infSli[0],
		Username: infSli[1],
		Password: infSli[2],
	})
	if err != nil {
		log.Fatal(err)
	}


	for v := range wc{

		// Create a new point batch
		bp, err := client.NewBatchPoints(client.BatchPointsConfig{
			Database:  infSli[3],
			Precision: infSli[4],
		})
		if err != nil {
			log.Fatal(err)
		}

		// Create a point and add to batch
		tags := map[string]string{"Path": v.Path, "Method":v.Method,"Scheme":v.Scheme,"Status":v.Status}
		fields := map[string]interface{}{
			"UpstreamTime":   v.UpstreamTime,
			"RequeststreamTime": v.RequestTime,
			"ByteSent":   v.BytesSent,
		}

		pt, err := client.NewPoint("nginx_log", tags, fields, v.TimeLocal)
		if err != nil {
			log.Fatal(err)
		}
		bp.AddPoint(pt)

		// Write the batch
		if err := c.Write(bp); err != nil {
			log.Fatal(err)
		}

		// Close client resources
		if err := c.Close(); err != nil {
			log.Fatal(err)
		}

		log.Println("写入成功！")
	}
}

func main() {

	var path, influxDsn string
	flag.StringVar(&path, "path","./tmp/access.log","read file path")
	flag.StringVar(&influxDsn, "influxDsn","http://127.0.0.1:8086@root@root@log@s","influx data source")
	flag.Parse()

	r := &ReadFromFile{path,}
	w := &WriteToInfluxDB{influxDsn,}

	lp := &LogProcess{
		rc: make(chan []byte),
		wc: make(chan *Message),

		//注入
		read:  r,
		write: w,
	}

	//并发执行
	go lp.read.Read(lp.rc)
	go lp.Process()
	go lp.write.Write(lp.wc)

	time.Sleep(30 * time.Second)
}
