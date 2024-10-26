package main

import (
	"context"
	"fmt"
	"github.com/olivere/elastic/v7"
	"log"
	"reflect"
	"time"
)

// Elasticsearch数据库信息
const (
	es_url       = "http://192.168.10.17:9200"
	es_user      = "alarm"
	es_password  = "Yang@ysh"
	index_patten = "javalogs-"                 // 索引前缀，-后面为日期
	kibana_url   = "http://192.168.10.17:5601" // 替换为你的 Kibana 地址
)

// Alertmanager和告警相关配置
const (
	alertmanager_url = "http://192.168.10.17:9094"
	alarm_threshold  = 10
	check_interval
	wecom_token = "e6df302f-0c6e-433d-addb-e8757e55d279"
)

type JavaLog struct {
	Timestamp time.Time `json:"@timestamp"` // 假设时间戳字段名为 "@timestamp"
	Message   string    `json:"message"`    // 假设日志消息字段名为 "message"
}

func main() {
	client, err := elastic.NewClient(elastic.SetURL(es_url))
	if err != nil {
		log.Fatalf("错误：创建Elasticsearch客户端失败：%v", err)
	}

	// 获取当前时间和前一分钟时间
	now := time.Now()
	before := now.Add(-1 * time.Minute)
	fmt.Println(now, "-----", before)
	// 构建查询
	query := elastic.NewBoolQuery().
		Must(elastic.NewRangeQuery("@timestamp").Gte(before).Lte(now),
			elastic.NewMatchQuery("message", "error"))

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

	// 处理查询结果
	fmt.Printf("找到 %d 条日志\n", searchResult.TotalHits())

	// 用于计数和存储前三条消息
	count := 0

	for _, item := range searchResult.Each(reflect.TypeOf(JavaLog{})) {
		logEntry := item.(JavaLog)
		// 输出前三条错误消息
		if count < 3 {
			fmt.Printf("第 %d 条错误消息：%s\n", count+1, logEntry.Message)
			count++
		}
	}

	if count == 0 {
		fmt.Println("没有找到错误消息")
	}

	// 构建 Kibana 查询面板链接
	kibanaLink := fmt.Sprintf("%s/app/discover#/?_g=(filters:!(),refreshInterval:(pause:!t,value:0),time:(from:'%s',to:'%s'))&_a=(columns:!('@timestamp',message),filters:!(),index:%s,interval:auto,query:(language:kuery,query:''),sort:!(!('@timestamp',desc)))",
		kibana_url, before.UTC().Format("2006-01-02T15:04:05.000Z"), now.UTC().Format("2006-01-02T15:04:05.000Z"), indexName)
	fmt.Println("Kibana 查询面板链接:", kibanaLink)

}
