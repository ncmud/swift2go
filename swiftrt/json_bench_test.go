package swiftrt

import "testing"

type benchItem struct {
	Name   string  `swift:"name"`
	Weight float64 `swift:"weight"`
	Count  int     `swift:"count"`
}

type benchRecord struct {
	ID       int64            `swift:"id"`
	Title    string           `swift:"title"`
	Score    float64          `swift:"score"`
	Active   bool             `swift:"active"`
	Nickname *string          `swift:"nickname"`
	Tags     []string         `swift:"tags"`
	Stats    map[string]int   `swift:"stats"`
	Items    []benchItem      `swift:"items"`
	Extra    map[string][]int `swift:"extra"`
}

func newBenchRecord() benchRecord {
	nick := "the brave/bold"
	items := make([]benchItem, 16)
	for i := range items {
		items[i] = benchItem{Name: "item \"" + string(rune('a'+i)) + "\"", Weight: float64(i) * 1.25, Count: i * 7}
	}
	return benchRecord{
		ID: 9223372036854775807, Title: "雪 snow <and> ash", Score: 0.30000000000000004, Active: true,
		Nickname: &nick, Tags: []string{"one", "two", "three", "four"},
		Stats: map[string]int{"str": 18, "dex": 14, "con": 16, "int": 9, "wis": 11, "cha": 8},
		Items: items, Extra: map[string][]int{"a": {1, 2, 3}, "b": {}},
	}
}

func BenchmarkYYJSONEncode(b *testing.B) {
	r := newBenchRecord()
	enc := YYJSONEncoder{WriteOptions: YYJSONWriteOptionsEscapeSlashes}
	b.ReportAllocs()
	for b.Loop() {
		_ = YYJSONEncoderEncode(enc, r)
	}
}

func BenchmarkJSONEncoderEncode(b *testing.B) {
	r := newBenchRecord()
	enc := JSONEncoderInit()
	b.ReportAllocs()
	for b.Loop() {
		_ = JSONEncoderEncode(enc, r)
	}
}

func BenchmarkYYJSONDecode(b *testing.B) {
	data := YYJSONEncoderEncode(YYJSONEncoder{}, newBenchRecord())
	b.ReportAllocs()
	for b.Loop() {
		_ = YYJSONDecoderDecodeFrom(YYJSONDecoder{}, TypeT[benchRecord]{}, data)
	}
}

func BenchmarkJSONDecoderDecode(b *testing.B) {
	data := YYJSONEncoderEncode(YYJSONEncoder{}, newBenchRecord())
	b.ReportAllocs()
	for b.Loop() {
		_ = JSONDecoderDecodeFrom(JSONDecoderInit(), TypeT[benchRecord]{}, data)
	}
}
