package core_test

import (
	"bufio"
	"bytes"
	"fmt"
	"github.com/go-test/deep"
	. "github.com/mshaverdo/radish/core"
	"io/ioutil"
	"math"
	"math/rand"
	"os"
	"reflect"
	"sort"
	"sync"
	"testing"
	"time"
)

func getSampleDataStorageHash() map[string]*Item {
	return map[string]*Item{
		"bytes": NewItemBytes([]byte("Призрак бродит по Европе - призрак коммунизма.")),
		"dict": NewItemDict(map[string][]byte{
			"banana": []byte("mama"),
			"測試":     []byte("別れ、比類のない"),
		}),
		"list": NewItemList([][]byte{
			[]byte("Abba"),
			[]byte("Rammstein"),
			[]byte("KMFDM"),
		}),
		"測": NewItemBytes([]byte("幽霊はヨーロッパを追いかけています - 共産主義の幽霊")),
	}
}

func TestStorageHash_Get(t *testing.T) {
	data := getSampleDataStorageHash()
	e := NewStorageHash()
	e.SetData(data)

	for key, item := range data {
		got := e.Get(key)
		if got != item {
			t.Errorf("Get(%q): got %p want %p (values: %q, %q)", key, got, item, got, item)
		}
	}
}

func TestStorageHash_GetSubmap(t *testing.T) {
	data := getSampleDataStorageHash()

	tests := []struct {
		keys []string
		want map[string]*Item
	}{
		{
			[]string{"bytes", "dict", "測", "404"},
			map[string]*Item{"bytes": data["bytes"], "dict": data["dict"], "測": data["測"]},
		},
	}

	e := NewStorageHash()
	e.SetData(data)

	for _, tst := range tests {
		got := e.GetSubmap(tst.keys)
		if !reflect.DeepEqual(got, tst.want) {
			t.Errorf("GetSubmap(%q): \ngot:%v\n\nwant:%v", tst.keys, got, tst.want)
		}
	}
}

func TestStorageHash_AddOrReplaceOne(t *testing.T) {
	tests := map[string]*Item{
		"測試": NewItemBytes([]byte("value of 測試")), "list": NewItemBytes([]byte("value of list")),
	}
	data := getSampleDataStorageHash()
	e := NewStorageHash()
	e.SetData(data)

	for key, item := range tests {
		e.AddOrReplaceOne(key, item)
		got := e.Get(key)
		if got != item {
			t.Errorf("Get(%q): got %p want %p (values: %q, %q)", key, got, item, got, item)
		}
	}
}

func TestStorageHash_Keys(t *testing.T) {
	data := getSampleDataStorageHash()
	e := NewStorageHash()
	e.SetData(data)

	var want []string
	for key := range data {
		want = append(want, key)
	}

	got := e.Keys()
	sort.Strings(got)
	sort.Strings(want)

	if diff := deep.Equal(got, want); diff != nil {
		t.Errorf("Keys(): %s\n\ngot:%v\n\nwant:%v", diff, got, want)
	}
}

func TestStorageHash_Del(t *testing.T) {
	tests := []struct {
		keys, want []string
	}{
		{[]string{"404", "測"}, []string{"bytes", "dict", "list"}},
		{[]string{"bytes", "dict"}, []string{"list"}},
	}

	data := getSampleDataStorageHash()
	e := NewStorageHash()
	e.SetData(data)

	for _, tst := range tests {
		e.Del(tst.keys)
		got := e.Keys()

		sort.Strings(got)
		sort.Strings(tst.want)
		if diff := deep.Equal(got, tst.want); diff != nil {
			t.Errorf("Del(): %s\n\ngot:%v\n\nwant:%v", diff, got, tst.want)
		}
	}
}

func TestStorageHash_DelSubmap(t *testing.T) {
	data := getSampleDataStorageHash()

	tests := []struct {
		submap    map[string]*Item
		wantCount int
		wantKeys  []string
	}{
		{
			map[string]*Item{"404": nil, "測": data["bytes"], "list": data["list"]},
			1,
			[]string{"bytes", "dict", "測"},
		},
		{
			map[string]*Item{"測": nil, "dict": data["dict"], "bytes": data["bytes"]},
			2,
			[]string{"測"},
		},
	}

	e := NewStorageHash()
	e.SetData(data)

	for _, tst := range tests {
		count := e.DelSubmap(tst.submap)
		got := e.Keys()

		sort.Strings(got)
		sort.Strings(tst.wantKeys)

		if count != tst.wantCount {
			t.Errorf("DelSubmap(%q) count: %d != %d", tst.submap, count, tst.wantCount)
		}

		if diff := deep.Equal(got, tst.wantKeys); diff != nil {
			t.Errorf("DelSubmap(%q): %s\n\ngot:%v\n\nwant:%v", tst.submap, diff, got, tst.wantKeys)
		}
	}
}

func TestStorageHash_concurrency(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}

	tests := [][]string{
		{"aa", "bb", "cc"},
		{"aa", "bb", "cc", "測", "測試"},
		{"測", "別れ、比類のない", "hhh"},
		{"aa", "bb", "cc", "測", "測試"},
	}

	var keys []string
	for i := 0; i < 100; i++ {
		keys = append(keys, fmt.Sprintf("%d", rand.Uint64()))
	}
	tests = append(tests, keys)

	e := NewStorageHash()
	var wg sync.WaitGroup
	for i := 0; i < 1000; i++ {
		wg.Add(1)
		go StorageHashWorker(&wg, e, tests)
	}

	wg.Wait()

	// Due to last operation of every StorageHashWorker is AddOrReplaceOne() for last keyset
	// after all workers done, only last keyset  should remain in the storage
	got := e.Keys()
	want := tests[len(tests)-1]
	sort.Strings(got)
	sort.Strings(want)
	if diff := deep.Equal(got, want); diff != nil {
		t.Errorf("Keys(): %s\n\ngot:%v\n\nwant:%v", diff, got, want)
	}
}

func StorageHashWorker(wg *sync.WaitGroup, e *StorageHash, tests [][]string) {
	var items map[string]*Item
	for _, tst := range tests {
		items = map[string]*Item{}
		for _, key := range tst {
			items[key] = NewItemBytes([]byte(time.Now().String()))
			e.Get(key)
		}

		for key, item := range items {
			e.AddOrReplaceOne(key, item)
		}
		e.GetSubmap(tst[1:3])
		e.Keys()
		e.DelSubmap(map[string]*Item{"404": nil, tst[0]: items[tst[0]], tst[1]: items[tst[1]]})
		e.Del(tst)
	}
	for key, item := range items {
		e.AddOrReplaceOne(key, item)
	}

	wg.Done()
}

func GetFilledStorageHash(n int) *StorageHash {
	s := NewStorageHash()
	for i := 0; i < n; i++ {
		key := fmt.Sprintf("key:%d", i)
		data := []byte(fmt.Sprintf("XXX"))
		var item *Item
		switch i % 3 {
		case 0:
			item = NewItemBytes(data)
		case 1:
			item = NewItemList([][]byte{data})
		case 2:
			item = NewItemDict(map[string][]byte{key: data})
		}
		s.AddOrReplaceOne(key, item)
	}

	return s
}

func TestStorageHash_PersistLoad(t *testing.T) {
	persisting := NewStorageHash()
	persisting.SetData(getSampleDataStorageHash())
	buf := bytes.NewBuffer(nil)

	err := persisting.Persist(buf, math.MaxInt64)
	if err != nil {
		t.Errorf("Failed to persist: %s", err)
	}

	loading := NewStorageHash()
	messageId, err := loading.Load(buf)

	if err != nil {
		t.Errorf("Failed to load: %s", err)
	}

	if messageId != math.MaxInt64 {
		t.Errorf("Invalid messageId: %d != %d", messageId, math.MaxInt64)
	}

	if !reflect.DeepEqual(loading.Data(), persisting.Data()) {
		t.Errorf("Persist/Load data mismatch: \ngot:%q\n\nwant:%q", loading.Data(), persisting.Data())
	}
}

func BenchmarkStorageHash_Persist(b *testing.B) {
	file, err := ioutil.TempFile("", "storage")
	w := bufio.NewWriter(file)

	if err != nil {
		b.Fatalf("Failed to create temp file: %s", err)
	}

	defer func() {
		name := file.Name()
		file.Close()
		os.Remove(name)
	}()

	s := GetFilledStorageHash(b.N)

	b.ResetTimer()
	s.Persist(w, 0)
	w.Flush()
}

func BenchmarkStorageHash_Load(b *testing.B) {
	file, err := ioutil.TempFile("", "storage")
	w := bufio.NewWriter(file)

	if err != nil {
		b.Fatalf("Failed to create temp file: %s", err)
	}

	defer func() {
		name := file.Name()
		file.Close()
		os.Remove(name)
	}()

	GetFilledStorageHash(b.N).Persist(w, 0)
	w.Flush()

	file.Seek(0, 0)
	r := bufio.NewReader(file)

	s := NewStorageHash()

	b.ResetTimer()
	s.Load(r)
}
