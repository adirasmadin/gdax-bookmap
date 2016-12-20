package websocket

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"strconv"

	"github.com/gorilla/websocket"

	"github.com/lian/gdax/orderbook"
)

func New(products []string, channel chan string) *Client {
	c := &Client{
		Updated:  channel,
		Products: []string{},
		Books:    map[string]*orderbook.Book{},
	}

	for _, name := range products {
		c.AddProduct(name)
	}

	return c
}

type Client struct {
	Updated  chan string
	Products []string
	Books    map[string]*orderbook.Book
	Socket   *websocket.Conn
}

func (c *Client) AddProduct(name string) {
	c.Products = append(c.Products, name)
	c.Books[name] = orderbook.New(name)
}

func (c *Client) Connect() {
	s, _, err := websocket.DefaultDialer.Dial("wss://ws-feed.gdax.com", nil)
	c.Socket = s

	if err != nil {
		log.Fatal("dial:", err)
	}

	buf, _ := json.Marshal(map[string]interface{}{"type": "subscribe", "product_ids": c.Products})
	err = c.Socket.WriteMessage(websocket.TextMessage, buf)
}

type PacketHeader struct {
	Type      string `json:"type"`
	Sequence  uint64 `json:"sequence"`
	ProductID string `json:"product_id"`
}

func (c *Client) BookChanged(book *orderbook.Book) {
	if c.Updated == nil {
		return
	}

	c.Updated <- book.ID
}

func (c *Client) HandleMessage(book *orderbook.Book, header PacketHeader, message []byte) {
	var data map[string]interface{}
	if err := json.Unmarshal(message, &data); err != nil {
		log.Fatal(err)
	}

	switch header.Type {
	case "received":
		// skip
		break
	case "open":
		price, _ := strconv.ParseFloat(data["price"].(string), 64)
		size, _ := strconv.ParseFloat(data["remaining_size"].(string), 64)

		book.Add(map[string]interface{}{
			"id":    data["order_id"].(string),
			"side":  data["side"].(string),
			"price": price,
			"size":  size,
		})
		c.BookChanged(book)

		break
	case "done":
		book.Remove(data["order_id"].(string))
		c.BookChanged(book)
		break
	case "match":
		price, _ := strconv.ParseFloat(data["price"].(string), 64)
		size, _ := strconv.ParseFloat(data["size"].(string), 64)

		book.Match(map[string]interface{}{
			"size":           size,
			"price":          price,
			"side":           data["side"].(string),
			"maker_order_id": data["maker_order_id"].(string),
			"taker_order_id": data["taker_order_id"].(string),
		})
		c.BookChanged(book)
		break
	case "change":
		if _, ok := book.OrderMap[data["order_id"].(string)]; !ok {
			// if we don't know about the order, it is a change message for a received order
		} else {
			// change messages are treated as match messages
			old_size, _ := strconv.ParseFloat(data["old_size"].(string), 64)
			new_size, _ := strconv.ParseFloat(data["new_size"].(string), 64)
			price, _ := strconv.ParseFloat(data["price"].(string), 64)
			size_delta := old_size - new_size

			book.Match(map[string]interface{}{
				"size":           size_delta,
				"price":          price,
				"side":           data["side"].(string),
				"maker_order_id": data["order_id"].(string),
			})
			c.BookChanged(book)
		}
		break
	}
}

func (c *Client) Run() {
	c.Connect()
	defer c.Socket.Close()

	for {
		msgType, message, err := c.Socket.ReadMessage()
		if err != nil {
			log.Println("read:", err)
			return
		}

		if msgType != websocket.TextMessage {
			continue
		}

		var header PacketHeader
		if err := json.Unmarshal(message, &header); err != nil {
			log.Println("header-parse:", err)
			return
		}

		var book *orderbook.Book
		var ok bool
		if book, ok = c.Books[header.ProductID]; !ok {
			log.Println("book not found")
			return
		}

		if book.SyncSequence == 0 {
			err := SyncBook(book)
			if err != nil {
				continue
			}
		}

		if header.Sequence <= book.SyncSequence {
			//fmt.Println("skip", header.Sequence)
			continue
		} else {
			if header.Sequence <= book.LastSequence {
				fmt.Println("skip_last_sequence", book.LastSequence, header.Sequence)
				os.Exit(1)
			} else {
				if header.Sequence == (book.LastSequence + 1) {
					book.LastSequence = header.Sequence
				} else {
					fmt.Println("sequence_gap", book.LastSequence, header.Sequence)
					os.Exit(1)
				}
			}
		}

		c.HandleMessage(book, header, message)
	}
}
