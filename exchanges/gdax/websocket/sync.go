package websocket

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"strconv"
	"time"

	"github.com/lian/gdax-bookmap/exchanges/gdax/orderbook"
)

func (c *Client) SyncBook(book *orderbook.Book) error {
	fmt.Println("sync", book.ID)

	full, err := FetchRawBook(3, book.ID)
	if err != nil {
		fmt.Println(err)
		return err
	}

	if seq, ok := full["sequence"]; ok {
		book.Clear()
		book.Sequence = uint64(seq.(float64))

		if bids, ok := full["bids"].([]interface{}); ok {
			for i := len(bids) - 1; i >= 0; i-- {
				data := bids[i].([]interface{})
				price, _ := strconv.ParseFloat(data[0].(string), 64)
				size, _ := strconv.ParseFloat(data[1].(string), 64)
				book.Add(map[string]interface{}{
					"id":    data[2].(string),
					"side":  "buy",
					"price": price,
					"size":  size,
				})
			}
		}
		if asks, ok := full["asks"].([]interface{}); ok {
			for i := len(asks) - 1; i >= 0; i-- {
				data := asks[i].([]interface{})
				price, _ := strconv.ParseFloat(data[0].(string), 64)
				size, _ := strconv.ParseFloat(data[1].(string), 64)
				book.Add(map[string]interface{}{
					"id":    data[2].(string),
					"side":  "sell",
					"price": price,
					"size":  size,
				})
			}
		}

		if c.dbEnabled {
			batch := c.BatchWrite[book.ID]
			now := time.Now()
			fmt.Println("STORE INIT SYNC", book.ID, batch.Count)
			c.WriteSync(batch, book, now)
		}
	}

	return nil
}

func FetchRawBook(level int, product string) (map[string]interface{}, error) {
	url := fmt.Sprintf("https://api.gdax.com/products/%s/book?level=%d", product, level)
	res, err := http.Get(url)
	if err != nil {
		return nil, err
	}
	body, err := ioutil.ReadAll(res.Body)
	if err != nil {
		return nil, err
	}

	var data map[string]interface{}
	err = json.Unmarshal(body, &data)
	if err != nil {
		return nil, err
	}

	return data, nil
}
