// 本地缓存：收件箱索引 + 每封正文；发件箱待发。
package mail

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

// Cache 本地邮件缓存根目录。
type Cache struct {
	Root string
}

func NewCache(root string) *Cache { return &Cache{Root: root} }

func (c *Cache) inboxIndex() string  { return filepath.Join(c.Root, "inbox.json") }
func (c *Cache) msgPath(uid uint32) string {
	return filepath.Join(c.Root, "messages", fmt.Sprintf("%d.json", uid))
}
func (c *Cache) outboxDir() string { return filepath.Join(c.Root, "outbox") }

// LoadInbox 读收件箱索引（按日期倒序）。
func (c *Cache) LoadInbox() (HeaderList, error) {
	data, err := os.ReadFile(c.inboxIndex())
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var list HeaderList
	if err := jsonUnmarshal(data, &list); err != nil {
		return nil, err
	}
	sort.Slice(list, func(i, j int) bool { return list[i].Date.After(list[j].Date) })
	return list, nil
}

// SaveInbox 写收件箱索引。
func (c *Cache) SaveInbox(list HeaderList) error {
	return atomicWrite(c.inboxIndex(), list)
}

// SaveMessage 存正文。
func (c *Cache) SaveMessage(m *Message) error {
	if err := os.MkdirAll(filepath.Join(c.Root, "messages"), 0o755); err != nil {
		return err
	}
	return atomicWrite(c.msgPath(m.UID), m)
}

// LoadMessage 读正文。
func (c *Cache) LoadMessage(uid uint32) (*Message, error) {
	data, err := os.ReadFile(c.msgPath(uid))
	if err != nil {
		return nil, err
	}
	var m Message
	if err := jsonUnmarshal(data, &m); err != nil {
		return nil, err
	}
	return &m, nil
}

// OutboxItem 待发邮件。
type OutboxItem struct {
	ID      string `json:"id"`
	To      string `json:"to"`
	Subject string `json:"subject"`
	Body    string `json:"body"`
	Error   string `json:"error,omitempty"`
}

// ListOutbox 列出待发（含失败）。
func (c *Cache) ListOutbox() ([]OutboxItem, error) {
	entries, err := os.ReadDir(c.outboxDir())
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var out []OutboxItem
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(c.outboxDir(), e.Name()))
		if err != nil {
			continue
		}
		var item OutboxItem
		if jsonUnmarshal(data, &item) == nil {
			out = append(out, item)
		}
	}
	return out, nil
}

// SaveOutbox 存待发。
func (c *Cache) SaveOutbox(item OutboxItem) error {
	if err := os.MkdirAll(c.outboxDir(), 0o755); err != nil {
		return err
	}
	if item.ID == "" {
		item.ID = strconv.FormatInt(int64(len(item.To)+len(item.Subject)), 36) + "-" + strconv.Itoa(int(nowUnix()))
	}
	return atomicWrite(filepath.Join(c.outboxDir(), item.ID+".json"), item)
}

// RemoveOutbox 删除待发。
func (c *Cache) RemoveOutbox(id string) error {
	return os.Remove(filepath.Join(c.outboxDir(), id+".json"))
}

func nowUnix() int64 {
	return time.Now().Unix()
}
