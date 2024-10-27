package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"github.com/olivere/elastic/v7"
	"log"
	"net/http"
	"reflect"
	"time"
)

// Elasticsearch数据库信息
const (
	es_url       = "http://192.168.10.17:9200"
	index_patten = "javalogs-"                 // 索引前缀，-后面为日期
	kibana_url   = "http://192.168.10.17:5601" // 替换为你的 Kibana 地址
)

// Alertmanager和告警相关配置
const (
	alertmanager_url = "http://192.168.10.17:9093"
	alarm_threshold  = 10
	check_interval   = 1
	wecom_token      = "e6df302f-0c6e-433d-addb-e8757e55d279"
)

type JavaLog struct {
	Timestamp time.Time `json:"@timestamp"` // 假设时间戳字段名为 "@timestamp"
	Message   string    `json:"message"`    // 假设日志消息字段名为 "message"
}

func main() {
	// 创建Elasticsearch客户端
	client, err := elastic.NewClient(elastic.SetURL(es_url))
	if err != nil {
		log.Fatalf("错误：创建Elasticsearch客户端失败：%v", err)
	}

	// 获取当前时间和间隔时间
	now := time.Now()
	before := now.Add(-check_interval * time.Minute)

	// 构建查询
	query := elastic.NewBoolQuery().Must(
		elastic.NewRangeQuery("@timestamp").Gte(before).Lte(now),
		elastic.NewMatchQuery("message", "error"),
	)

	// 执行查询
	today := time.Now().Format("2006-01-02")              // 格式化为 YYYY-MM-DD
	indexName := fmt.Sprintf("%s%s", index_patten, today) // 构建索引名称
	searchResult, err := client.Search().
		Index(indexName).
		Query(query).
		Do(context.Background())
	if err != nil {
		log.Fatalf("错误，从Elasticsearch获取数据失败: %v", err)
	}

	// 列出查询结果
	fmt.Printf("查询范围%s————%s\n", before, now)
	fmt.Printf("找到 %d 条日志\n", searchResult.TotalHits())

	// 用于计数和存储前三条消息
	count := 0
	var errorMessages []string
	for _, item := range searchResult.Each(reflect.TypeOf(JavaLog{})) {
		logEntry := item.(JavaLog)
		// 输出前三条错误消息
		if count < 3 {
			fmt.Printf("第 %d 条错误消息：%s\n", count+1, logEntry.Message)
			errorMessages = append(errorMessages, logEntry.Message)
			count++
		}
	}

	// 检查是否存在静默
	if is_silence(indexName) {
		log.Println("当前存在活动静默，跳过告警")
		return
	}

	// 构建 Kibana 查询面板链接
	kibanaLink := fmt.Sprintf("%s/app/discover#/?_g=(filters:!(),refreshInterval:(pause:!t,value:0),time:(from:'%s',to:'%s'))&_a=(columns:!('@timestamp',message),filters:!(),index:%s,interval:auto,query:(language:kuery,query:''),sort:!(!('@timestamp',desc)))",
		kibana_url, before.UTC().Format("2006-01-02T15:04:05.000Z"), now.UTC().Format("2006-01-02T15:04:05.000Z"), indexName)

	// 构建屏蔽链接
	silence_url := fmt.Sprint(alertmanager_url + "/#/silences/new?filter=%7Bpath%3D%22" + indexName + "%22%7D")

	// 判断
	if count < alarm_threshold {
		fmt.Printf("找到%d条日志，告警阈值为%d", count, alarm_threshold)
	} else if count >= alarm_threshold {
		// 如果有错误消息，构建完整的 message
		errorLogList := ""
		for i, msg := range errorMessages {
			errorLogList += fmt.Sprintf("- 第 %d 条：%s\n", i+1, msg)
		}
		message := fmt.Sprintf(
			"# Log Alert\n"+
				"## <font color=\"#ff0000\">【%s】返回error异常</font>\n "+
				"错误次数：%d \n "+
				"前三条错误消息：\n%s \n"+
				"- [点击查询告警时段日志](%s)\n"+
				"- [点击这里进行屏蔽](%s)",
			indexName, searchResult.TotalHits(), errorLogList, kibanaLink, silence_url)
		log.Println("--------------------发出的告警信息--------------------")
		log.Println(message)
		log.Println("--------------------告警信息结尾--------------------")
		sendWeCom(message)
	}
}

// 发送企微
func sendWeCom(message string) {
	webhook := fmt.Sprintf("https://qyapi.weixin.qq.com/cgi-bin/webhook/send?key=%s", wecom_token)
	params := map[string]interface{}{
		"msgtype": "markdown",
		"markdown": map[string]interface{}{
			"content": message,
		},
	}

	data, _ := json.Marshal(params)
	resp, err := http.Post(webhook, "application/json", bytes.NewBuffer(data))
	if err != nil {
		log.Printf("Failed to send WeCom message: %v", err)
		return
	}
	defer resp.Body.Close()

	var result map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&result)
	log.Printf("WeCom response: %v", result)

}

// 检查是否存在静默
func is_silence(indexName string) bool {
	url := fmt.Sprintf("%s/api/v2/silences?filter=path=%22javalogs-2024-10-27%22&active=true", alertmanager_url)
	silenceList, err := http.Get(url)
	if err != nil {
		log.Printf("Failed to check silence: %s", err)
		return false
	}
	defer silenceList.Body.Close()

	var silences []map[string]interface{}
	if err := json.NewDecoder(silenceList.Body).Decode(&silences); err != nil {
		log.Printf("Failed to decode response: %s", err)
		return false
	}

	for _, silence := range silences {
		if status, ok := silence["status"].(map[string]interface{}); ok {
			if state, ok := status["state"].(string); ok && state == "active" {
				return true
			}
		}
	}
	return false
}
