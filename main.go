package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"gopkg.in/yaml.v2"
	"io"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strings"
	"time"
)

type Config struct {
	AsusHostname string `yaml:"AsusHostname"`
	ID           string `yaml:"ID"`
	Token        string `yaml:"Token"`
	Domain       struct {
		Name         string `yaml:"Name"`
		RecordType   string `yaml:"RecordType"`
		LatestRecord string `yaml:"LatestRecord"`
		RecordLine   string `yaml:"RecordLine"`
	} `yaml:"Domain"`
}

type AlertDRRes struct {
	Status struct {
		Code      string `json:"code"`
		Message   string `json:"message"`
		CreatedAt string `json:"created_at"`
	} `json:"status"`
	Record struct {
		ID     int    `json:"id"`
		Name   string `json:"name"`
		Value  string `json:"value"`
		Status string `json:"status"`
	} `json:"record"`
}

type Records struct {
	Status struct {
		Code      string `json:"code"`
		Message   string `json:"message"`
		CreatedAt string `json:"created_at"`
	} `json:"status"`
	Domain struct {
		ID            string   `json:"id"`
		Name          string   `json:"name"`
		Punycode      string   `json:"punycode"`
		Grade         string   `json:"grade"`
		Owner         string   `json:"owner"`
		ExtStatus     string   `json:"ext_status"`
		TTL           int      `json:"ttl"`
		MinTTL        int      `json:"min_ttl"`
		DnspodNs      []string `json:"dnspod_ns"`
		Status        string   `json:"status"`
		CanHandleAtNs bool     `json:"can_handle_at_ns"`
	} `json:"domain"`
	Info struct {
		SubDomains  string `json:"sub_domains"`
		RecordTotal string `json:"record_total"`
		RecordsNum  string `json:"records_num"`
	} `json:"info"`
	Records []struct {
		ID            string `json:"id"`
		TTL           string `json:"ttl"`
		Value         string `json:"value"`
		Enabled       string `json:"enabled"`
		Status        string `json:"status"`
		UpdatedOn     string `json:"updated_on"`
		RecordTypeV1  string `json:"record_type_v1"`
		Name          string `json:"name"`
		Line          string `json:"line"`
		LineID        string `json:"line_id"`
		Type          string `json:"type"`
		Weight        any    `json:"weight"`
		MonitorStatus string `json:"monitor_status"`
		Remark        string `json:"remark"`
		UseAqb        string `json:"use_aqb"`
		Mx            string `json:"mx"`
		Hold          string `json:"hold,omitempty"`
	} `json:"records"`
}

type PublicIPAddr struct {
	Origin string `json:"origin"`
}

func main() {

	config, err := unmarshalConfig("config.yaml")
	if err != nil {
		return
	}

	var publicIP string
	if config.AsusHostname != "" {
		publicIP, err = getPublicIP2(config.AsusHostname)
	} else {
		publicIP, err = getPublicIP()
	}

	if err != nil {
		log(err.Error())
		return
	}

	if publicIP == config.Domain.LatestRecord {
		// log("公网ip地址未改变，无需更新")
		return
	}

	err = setDomainRecord(publicIP, config)
	if err != nil {
		log("IP解析记录更新失败：" + err.Error())
		return
	}

	log("IP解析记录更新成功！公网IP地址：" + publicIP)
	config.Domain.LatestRecord = publicIP

	configString, err := yaml.Marshal(&config)
	if err != nil {
		return
	}
	file, err := os.OpenFile("config.yaml", os.O_TRUNC|os.O_WRONLY, 0777)
	if err != nil {
		return
	}
	_, err = file.WriteString(string(configString))
	if err != nil {
		return
	}
	file.Close()
}

func getRecordID(token string, domain string, recordType string) (string, error) {
	interfaceUrl := `https://dnsapi.cn/Record.List`

	request, err := http.NewRequest("POST", interfaceUrl, strings.NewReader(
		fmt.Sprintf("domain=%s&login_token=%s", domain, token)),
	)
	if err != nil {
		return "", err
	}
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	request.Header.Set("Accept", "*/*")

	//proxyurl, _ := url.Parse("http://127.0.0.1:8888")
	//transport := &http.Transport{
	//	Proxy: http.ProxyURL(proxyurl),
	//}
	//resp, err := transport.RoundTrip(request)

	client := &http.Client{}
	resp, err := client.Do(request)
	if err != nil {
		return "", err
	}

	defer resp.Body.Close()
	var jsonData Records
	err = json.NewDecoder(resp.Body).Decode(&jsonData)
	if err != nil {
		return "", err
	}

	if jsonData.Status.Code != "1" {
		return "", errors.New("记录枚举API返回结果异常！")
	}

	var id string = ""
	for _, record := range jsonData.Records {

		if record.Type == recordType {
			id = record.ID
			break
		}

	}

	if id == "" {
		return id, errors.New("未找到A记录ID！")
	}

	return id, nil

}

func setDomainRecord(publicIP string, config Config) error {
	var token string = config.ID + "," + config.Token
	recordID, err := getRecordID(token, config.Domain.Name, config.Domain.RecordType)
	if err != nil {
		return errors.New("获取记录ID过程错误！" + err.Error())
	}

	var interfaceUrl = `https://dnsapi.cn/Record.Modify`
	data := "domain=%s&login_token=%s&record_id=%s&record_line=%s&record_type=%s&value=%s"
	request, err := http.NewRequest("POST", interfaceUrl, strings.NewReader(
		fmt.Sprintf(data, config.Domain.Name, token, recordID, config.Domain.RecordLine,
			config.Domain.RecordType, publicIP)))
	if err != nil {
		return err
	}
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	request.Header.Set("Accept", "*/*")

	client := &http.Client{}
	resp, err := client.Do(request)
	if err != nil {
		return err
	}

	defer resp.Body.Close()
	var response AlertDRRes

	err = json.NewDecoder(resp.Body).Decode(&response)
	if err != nil {
		return err
	}

	if response.Status.Code == "1" {
		return nil
	} else {
		return errors.New(response.Status.Message)
	}

}

func unmarshalConfig(filename string) (Config, error) {
	var config Config

	configFile, err := os.ReadFile(filename)
	if err != nil {
		log("配置文件打开失败！" + err.Error())
		return config, err
	}

	err = yaml.Unmarshal(configFile, &config)
	if err != nil {
		log("解析配置文件失败！" + err.Error())
		return config, err
	}
	return config, err
}

func getPublicIP() (string, error) {

	interfaceUrl := "https://httpbin.org/ip"

	// 发送 HTTP GET 请求获取公网 IP
	resp, err := http.Get(interfaceUrl)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	// 读取响应内容
	var ipaddr PublicIPAddr
	err = json.NewDecoder(resp.Body).Decode(&ipaddr)
	if err != nil {
		return "", err
	}

	ip := string(ipaddr.Origin)
	return ip, nil

}

func getPublicIP2(hostname string) (string, error) {

	data := url.Values{"hostname": {hostname}}
	request, err := http.NewRequest(
		"POST", "https://iplookup.asus.com/nslookup.php",
		bytes.NewBufferString(data.Encode()))
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	if err != nil {
		return "", errors.New("创建请求失败！" + err.Error())
	}

	client := &http.Client{}
	resp, err := client.Do(request)

	if err != nil {
		return "", errors.New("公网IP请求发送失败！" + err.Error())
	}

	defer func(Body io.ReadCloser) {
		err := Body.Close()
		if err != nil {
			log("公网IP请求返回内容读取失败！" + err.Error())
		}
	}(resp.Body)

	body, err := io.ReadAll(resp.Body)

	if err != nil {
		return "", errors.New("公网IP请求返回内容读取失败！" + err.Error())
	}

	re := regexp.MustCompile(`\d+\.\d+\.\d+\.\d+`)
	match := re.FindAllString(string(body), 1)

	if len(match) == 0 {
		return "", errors.New("在返回结果中无法找到IP地址！" + err.Error())
	}

	return match[0], nil
}

func log(msg string) {

	_ = os.MkdirAll("log", 0777)

	file, err := os.OpenFile(
		".\\log\\"+time.Now().Format("2006-01-02")+".log",
		os.O_APPEND|os.O_CREATE, 0777)

	if err != nil {
		fmt.Println("log文件打开失败！")
		return
	}

	write := time.Now().Format("2006-01-02 15:04:05") + "  " + msg + "\n"
	_, err = file.WriteString(write)
	if err != nil {
		return
	}
	err = file.Close()
	if err != nil {
		return
	}
	fmt.Println(write)

}
