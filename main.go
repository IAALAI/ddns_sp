package main

import (
	"bufio"
	"context"
	"flag"
	"fmt"
	"log"
	"math/rand"
	"net"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/cloudflare/cloudflare-go"
	"github.com/goccy/go-json"
)

// type

type ConfigStruct struct {
	Host    string
	Port    int
	KeyFile string
	Bind    bool
}

type ClientStruct struct {
	ApiKey      string `json:"api_key"`
	Email       string `json:"email"`
	ZoneId      string `json:"zone_id"`
	DNSRecordId string `json:"dns_record_id"`
}

// type of result

type Meta struct {
	AutoAdded bool   `json:"auto_added"`
	Source    string `json:"source"`
}

type Result struct {
	Content    string   `json:"content"`
	Name       string   `json:"name"`
	Proxied    bool     `json:"proxied"`
	Type       string   `json:"type"`
	Comment    string   `json:"comment"`
	CreatedOn  string   `json:"created_on"`
	ID         string   `json:"id"`
	Locked     bool     `json:"locked"`
	Meta       Meta     `json:"meta"`
	ModifiedOn string   `json:"modified_on"`
	Proxiable  bool     `json:"proxiable"`
	Tags       []string `json:"tags"`
	TTL        int      `json:"ttl"`
}

type Response struct {
	Errors   []interface{} `json:"errors"`
	Messages []interface{} `json:"messages"`
	Success  bool          `json:"success"`
	Result   Result        `json:"result"`
}

// global variable and tool function

var config = ConfigStruct{}
var client_option = ClientStruct{}
var now_ip, cf_ip string
var fatal_error = false
var server_error = false
var random = rand.New(rand.NewSource(time.Now().UnixNano()))
var zone_name string
var api *cloudflare.API
var ctx = context.Background()

func wait() {
	time.Sleep(time.Duration(random.Intn(6)+6) * time.Second)
}

func handleConnection(conn net.Conn) {
	defer conn.Close()
	reader := bufio.NewReader(conn)
	// Verify Data, V = receivedTimestamp / 3
	VStr, _ := reader.ReadString('\n')
	V, _ := strconv.ParseInt(strings.TrimSpace(VStr), 10, 64)
	N := time.Now().UnixMilli() / 3
	if N-V > 1000 {
		fmt.Println("Invalid Verify Data: ", N, V, N-V)
		return
	}
	fmt.Println("Accept connect from:", conn.RemoteAddr().String())
	conn.Write([]byte(conn.RemoteAddr().(*net.TCPAddr).IP.String() + "\n"))
	for {
		message, err := reader.ReadString('\n')
		if err != nil {
			fmt.Println("connect is disconnect")
			return
		}
		conn.Write([]byte(message))
	}
}

// business code

func server() {
	port := strconv.Itoa(config.Port)
	listener, err := net.Listen("tcp", config.Host+":"+port)
	if err != nil {
		// fmt.Errorf("Failed to listen on port " + port + ": " + err.Error())
		log.Println("Failed to listen on port " + port + ": " + err.Error())
		return
	}
	defer listener.Close()
	fmt.Println("listening on " + config.Host + ":" + port)
	for {
		conn, err := listener.Accept()
		if err != nil {
			fmt.Println("Error accepting: ", err.Error())
			continue
		}
		go handleConnection(conn)
	}
}

func changeIP() error {
	data := cloudflare.UpdateDNSRecordParams{
		ID:      client_option.DNSRecordId,
		Name:    zone_name,
		Content: now_ip,
		Type:    "A",
		TTL:     1,
	}
	response, err := api.UpdateDNSRecord(ctx, cloudflare.ZoneIdentifier(client_option.ZoneId), data)
	if err != nil {
		return err
	}
	fmt.Println("success,now ip is: " + response.Content)
	return nil
}

func getCFIP() error {
	response, err := api.GetDNSRecord(ctx, cloudflare.ZoneIdentifier(client_option.ZoneId), client_option.DNSRecordId)
	cf_ip = response.Content
	zone_name = response.Name
	fmt.Println("get CF IP:", cf_ip)
	return err
}

func connectto() {
	/*
		todo have bug
		我感觉这里是不需要进行返回的
		因为如果连接正常是应该保持连接的
		只有出错才需要返回
		但是返回之后我也要求无限重连
	*/
	port := strconv.Itoa(config.Port)
	conn, err := net.Dial("tcp", config.Host+":"+port)
	if err != nil {
		log.Println("Failed to connect to " + config.Host + ":" + port + ": " + err.Error())
		return
	} else {
		fmt.Println("connect to " + config.Host + ":" + port)
	}
	defer conn.Close()
	conn.Write([]byte(strconv.Itoa(int(time.Now().UnixMilli()/3)) + "\n"))
	now_ip, _ = bufio.NewReader(conn).ReadString('\n')
	if now_ip != cf_ip {
		fmt.Println("IP different")
		err := changeIP()
		if err != nil {
			// 连不上 CF ,直接死刑
			fatal_error = true
			log.Println(err)
			return
		}
	}
	for {
		time.Sleep(time.Duration(random.Intn(6)+6) * time.Second)
		_, err = conn.Write([]byte(string(time.Now().UnixNano()%1000) + "\n"))
		if err != nil {
			fmt.Println("Error writing:", err)
			return
		}
		response, err := bufio.NewReader(conn).ReadString('\n')
		if err != nil || response == "" || response == "EOF" {
			fmt.Println("Accept disconnect:", err)
			return
		}
	}
}

func client() {
	// read key file
	file, err := os.ReadFile(config.KeyFile)
	if err != nil {
		log.Println("Failed load key :" + config.KeyFile + ": " + err.Error())
		return
	}
	json.Unmarshal(file, &client_option)
	api, err = cloudflare.New(client_option.ApiKey, client_option.Email)
	if err != nil || getCFIP() != nil {
		// 没有权限,这还跑啥
		log.Println(err)
		return
	}

	for !fatal_error {
		/*
			如果是连接服务器失败就进行重试
			因为服务器连接失败有可能是因为网络波动丢包
			如果是客户端异常,直接杀死
		*/
		if server_error {
			getCFIP()
			time.Sleep(time.Second * 12)
			server_error = false
		}

		connectto()
	}
}

// run

func main() {
	// parse flags
	flag.StringVar(&config.Host, "host", "0.0.0.0", "host to listen on")
	flag.IntVar(&config.Port, "port", 51100, "port to listen on")
	flag.StringVar(&config.KeyFile, "key", "apikey.json", "key file to use")
	flag.BoolVar(&config.Bind, "bind", false, "bind to all interfaces")
	flag.Parse()

	if config.Bind {
		server()
	} else {
		client()
	}
}
