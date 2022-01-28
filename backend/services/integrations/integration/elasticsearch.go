package integration

import (
	"bytes"
	"context"
	b64 "encoding/base64"
	"encoding/json"
	"fmt"
	elasticlib "github.com/elastic/go-elasticsearch/v7"
	"log"
	"reflect"
	"strconv"
	"time"

	"openreplay/backend/pkg/messages"
	"openreplay/backend/pkg/utime"
)

type elasticsearch struct {
	Host     string
	Port     json.Number
	ApiKeyId string //`json:"api_key_id"`
	ApiKey   string //`json:"api_key"`
	Indexes  string
}

type elasticsearchLog struct {
	Message string
	Time    time.Time `json:"utc_time"` // Should be parsed automatically from RFC3339
}

//map[string]interface{}
// SearchResult represents the result of the search operation
//type elasticResponse struct {
//	Hits struct {
//		Hits []struct {
//			Id     string          `json:"_id"`
//			Source json.RawMessage `json:"_source"`
//		} `json:"hits"`
//	} `json:"hits"`
//	ScrollId string `json:"_scroll_id"`
//}


func (es *elasticsearch) Request(c *client) error {
	address := es.Host + ":" + es.Port.String()
	apiKey := b64.StdEncoding.EncodeToString([]byte(es.ApiKeyId + ":" + es.ApiKey))
	cfg := elasticlib.Config{
		Addresses: []string{
			address,
		},
		//Username: es.ApiKeyId,
		//Password: es.ApiKey,
		APIKey: apiKey,
	}
	esC, err := elasticlib.NewClient(cfg)

	if err != nil {
		log.Println("Error while creating new ES client")
		log.Println(err)
		return err
	}
	// TODO: ping/versions/ client host check
	//  res0, err := esC.Info()
	// if err != nil {
	//     log.Printf("ELASTIC Error getting info: %s", err)
	//  }
	//  defer res0.Body.Close()
	//  // Check response status
	//  if res0.IsError() {
	//    log.Printf("ELASTIC Error: %s", res0.String())
	//  }
	//  log.Printf("ELASTIC  Info: %v ", res0.String())

	gteTs := c.getLastMessageTimestamp() + 1000 // Sec or millisec to add ?
	log.Printf("gteTs: %v ", gteTs)
	var buf bytes.Buffer
	query := map[string]interface{}{
		"query": map[string]interface{}{
			"bool": map[string]interface{}{
				"filter": []map[string]interface{}{
					map[string]interface{}{
						"match": map[string]interface{}{
							"message": map[string]interface{}{
								"query": "openReplaySessionToken=", // asayer_session_id=
							},
						},
					},
					map[string]interface{}{
						"range": map[string]interface{}{
							"utc_time": map[string]interface{}{
								"gte": strconv.FormatUint(gteTs, 10),
								//"gte": gteTs,
								"lte": "now",
							},
						},
					},
					map[string]interface{}{
						"term": map[string]interface{}{
							"tags": "error",
						},
					},
				},
			},
		},
	}

	if err := json.NewEncoder(&buf).Encode(query); err != nil {
		return fmt.Errorf("Error encoding the query: %s", err)
	}

	res, err := esC.Search(
		esC.Search.WithContext(context.Background()),
		esC.Search.WithIndex(es.Indexes),
		esC.Search.WithSize(1000),
		esC.Search.WithScroll(time.Minute*2),
		esC.Search.WithBody(&buf),
		esC.Search.WithSort("utc_time:asc"),
	)
	if err != nil {
		return fmt.Errorf("Error getting response: %s", err)
	}
	defer res.Body.Close()
	if res.IsError() {
		var e map[string]interface{}
		if err := json.NewDecoder(res.Body).Decode(&e); err != nil {
			log.Printf("Error parsing the Error response body: %v\n", err)
			return fmt.Errorf("Error parsing the Error response body: %v", err)
		} else {
			log.Printf("Elasticsearch Error [%s] %s: %s\n",
				res.Status(),
				e["error"], //.(map[string]interface{})["type"],
				e["error"], //.(map[string]interface{})["reason"],
			)
			return fmt.Errorf("Elasticsearch Error [%s] %s: %s",
				res.Status(),
				e["error"], //.(map[string]interface{})["type"],
				e["error"], //.(map[string]interface{})["reason"],
			)
		}
	}

	for {
		var esResp map[string]interface{}
		if err := json.NewDecoder(res.Body).Decode(&esResp); err != nil {
			log.Fatalf("Error parsing raw response body: %s", err)

			// If no error, then convert response to a map[string]interface
		} else {
			log.Println("mapResp TYPE:", reflect.TypeOf(esResp), "\n")
			log.Println(esResp)
		}

		//var esResp elasticResponse
		//if err := json.NewDecoder(res.Body).Decode(&esResp); err != nil {
		//	log.Printf("Error parsing the response body: %s\n", err)
		//	return fmt.Errorf("Error parsing the response body: %s", err)
		//}

		log.Printf("parsed response: %v\n", esResp)
		if _, ok := esResp["hits"]; ok {
			log.Println("Hits not found")
			break
		}
		hits:=esResp["hits"].(map[string]interface{})["hits"].([]interface{})
		for _, hit := range hits{

			// Parse the attributes/fields of the document
			doc := hit.(map[string]interface{})
			source := doc["_source"]

			var esLog elasticsearchLog
			esLog=source.(elasticsearchLog)
			log.Println("doc _source:", reflect.TypeOf(source))

			// Get the document's _id and print it out along with _source data
			docID := doc["_id"]
			log.Println("docID:", docID)
			log.Println("_source:", source, "\n")
		//}
		////if len(esResp.Hits.Hits) == 0 {
		//if len(hits) == 0 {
		//	break
		//}
		//
		//for _, hit := range esResp.Hits.Hits {
		//	var esLog elasticsearchLog
			//if err = json.Unmarshal(hit.Source, &esLog); err != nil {
			//if err = json.Unmarshal(source, &esLog); err != nil {
			//	log.Printf("Error unmarshalling the response source: %s\n", err)
			//	c.errChan <- err
			//	continue
			//}
			token, err := GetToken(esLog.Message)
			if err != nil {
				log.Printf("Error generating token: %s\n", err)
				c.errChan <- err
				continue
			}
			//parsedTime, err := time.Parse(time.RFC3339, esLog.Timestamp)
			//if err != nil {
			//	c.errChan <- err
			//	continue
			//}
			timestamp := uint64(utime.ToMilliseconds(esLog.Time))
			c.setLastMessageTimestamp(timestamp)
			c.evChan <- &SessionErrorEvent{
				//SessionID: sessionID,
				Token: token,
				RawErrorEvent: &messages.RawErrorEvent{
					Source:    "elasticsearch",
					Timestamp: timestamp,
					//Name:      hit.Id, // sure?
					Name:      fmt.Sprintf("%v", docID), // sure?
					//Payload:   string(hit.Source),
					Payload:   fmt.Sprintf("%v", source),
				},
			}
		}
		if _, ok := esResp["_scroll_id"]; ok {
			log.Println("_scroll_id not found")
			break
		}
		scrollId := esResp["_scroll_id"]
		res, err = esC.Scroll(
			esC.Scroll.WithContext(context.Background()),
			//esC.Scroll.WithScrollID(esResp.ScrollId),
			esC.Scroll.WithScrollID(fmt.Sprintf("%v", scrollId)),
			esC.Scroll.WithScroll(time.Minute*2),
		)
		if err != nil {
			return fmt.Errorf("Error getting scroll response: %s", err)
		}
		defer res.Body.Close()
		if res.IsError() {
			var e map[string]interface{}
			if err := json.NewDecoder(res.Body).Decode(&e); err != nil {
				return fmt.Errorf("Error parsing the response body: %v", err)
			} else {
				return fmt.Errorf("Elasticsearch [%s] %s: %s",
					res.Status(),
					e["error"], //.(map[string]interface{})["type"],
					e["error"], //.(map[string]interface{})["reason"],
				)
			}
		}
	}
	return nil
}
