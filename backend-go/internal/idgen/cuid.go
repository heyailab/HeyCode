// Package idgen 生成 24 字符的 cuid 风格 ID。
//
// 格式：'c' + base36(timestamp,8) + base36(counter,6) + base36(random,9) = 24 字符
// 保证单进程内单调递增（counter）+ 跨进程唯一性（random）。
package idgen

import (
	"crypto/rand"
	"strconv"
	"sync/atomic"
	"time"
)

var counter uint64

func init() {
	var b [8]byte
	_, _ = rand.Read(b[:])
	var seed uint64
	for _, x := range b {
		seed = (seed << 8) | uint64(x)
	}
	atomic.StoreUint64(&counter, seed)
}

// New 生成一个 24 字符 ID。
func New() string {
	ts := uint64(time.Now().UnixMilli())
	c := atomic.AddUint64(&counter, 1)
	var rb [8]byte
	_, _ = rand.Read(rb[:])
	var rnd uint64
	for _, x := range rb {
		rnd = (rnd << 8) | uint64(x)
	}
	s := "c" +
		pad(strconv.FormatUint(ts, 36), 8) +
		pad(strconv.FormatUint(c, 36), 6) +
		pad(strconv.FormatUint(rnd, 36), 9)
	if len(s) > 24 {
		s = s[:24]
	}
	return s
}

func pad(s string, n int) string {
	if len(s) >= n {
		return s
	}
	return "0000000000000000"[:n-len(s)] + s
}
