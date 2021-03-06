package core_test

import (
	"fmt"
	"github.com/go-test/deep"
	. "github.com/mshaverdo/radish/core"
	"math/rand"
	"sort"
	"sync"
	"testing"
	"time"
)

type MockStorage struct {
	data map[string]*Item
}

func getSampleDataCore() map[string]*Item {
	expiredItem := NewItemBytes([]byte("Expired"))
	expiredItem.SetMilliTtl(1)
	time.Sleep(1 * time.Millisecond)

	expireLaterItem := NewItemBytes([]byte("Призрак бродит по Европе - призрак коммунизма."))
	expireLaterItem.SetTtl(1000)

	return map[string]*Item{
		"bytes": expireLaterItem,
		"dict": NewItemDict(map[string][]byte{
			"banana": []byte("mama"),
			"測試":     []byte("別れ、比類のない"),
		}),
		"list": NewItemList([][]byte{
			//IMPORTANT: by proto, HEAD of the list has index 0, but in the slice storage it is the LAST element of the slice
			[]byte("Abba"),
			[]byte("Rammstein"),
			[]byte("KMFDM"),
		}),
		"測":       NewItemBytes([]byte("幽霊はヨーロッパを追いかけています - 共産主義の幽霊")),
		"expired": expiredItem,
	}
}

func NewMockStorage() *MockStorage {
	return &MockStorage{data: getSampleDataCore()}
}

func (e *MockStorage) Get(key string) (item *Item) {
	return e.data[key]
}

func (e *MockStorage) Keys() (keys []string) {
	keys = make([]string, 0, len(e.data))
	for k := range e.data {
		keys = append(keys, k)
	}

	return keys
}

func (e *MockStorage) AddOrReplaceOne(key string, item *Item) {
	e.data[key] = item
}

func (e *MockStorage) Del(keys []string) (count int) {
	for _, k := range keys {
		if _, ok := e.data[k]; ok {
			count++
		}

		delete(e.data, k)
	}

	return count
}

func (e *MockStorage) GetSubmap(keys []string) (submap map[string]*Item) {
	submap = make(map[string]*Item, len(keys))

	for _, key := range keys {
		if item, ok := e.data[key]; ok {
			submap[key] = item
		}
	}

	return submap
}

func (e *MockStorage) DelSubmap(submap map[string]*Item) (count int) {
	for key, item := range submap {
		if existingItem, ok := e.data[key]; ok && existingItem == item {
			count++
			delete(e.data, key)
		}
	}

	return count
}

/////////////////////  Tests  ///////////////////////////

func TestCore_Keys(t *testing.T) {
	tests := []struct {
		pattern string
		want    []string
	}{
		{"*", []string{"bytes", "dict", "list", "測"}},
		{"bytes", []string{"bytes"}},
		{"*i*", []string{"dict", "list"}},
	}

	c := New(NewMockStorage())

	for _, tst := range tests {
		got := c.Keys(tst.pattern)
		sort.Strings(got)
		sort.Strings(tst.want)

		if diff := deep.Equal(got, tst.want); diff != nil {
			t.Errorf("Keys(%q): %s\n\ngot:%v\n\nwant:%v", tst.pattern, diff, got, tst.want)
		}
	}
}

func TestCore_Get(t *testing.T) {
	tests := []struct {
		key  string
		err  error
		want string
	}{
		{"bytes", nil, "Призрак бродит по Европе - призрак коммунизма."},
		{"測", nil, "幽霊はヨーロッパを追いかけています - 共産主義の幽霊"},
		{"404", ErrNotFound, ""},
		{"expired", ErrNotFound, ""},
		{"dict", ErrWrongType, ""},
	}

	c := New(NewMockStorage())

	for _, tst := range tests {
		got, err := c.Get(tst.key)
		if err != tst.err {
			t.Errorf("Get(%q) err: %q != %q", tst.key, err, tst.err)
		}
		if string(got) != tst.want {
			t.Errorf("Get(%q) err: %q != %q", tst.key, string(got), tst.want)
		}
	}
}

func TestCore_Set(t *testing.T) {
	tests := []struct {
		key   string
		value string
	}{
		{"bytes", "Ктулху фхтагн!"},
		{"new 測", "共産主義の幽霊"},
		{"expired", "not expired"},
	}

	c := New(NewMockStorage())

	for _, tst := range tests {
		c.Set(tst.key, []byte(tst.value))
		got, err := c.Get(tst.key)
		if err != nil {
			t.Errorf("Set(%q) err: %q != nil", tst.key, err)
		}
		if string(got) != tst.value {
			t.Errorf("Set(%q) got: %q != %q", tst.key, string(got), tst.value)
		}
	}
}

func TestCore_Del(t *testing.T) {
	tests := []struct {
		keys []string
		want []string
	}{
		{[]string{"bytes", "list", "404"}, []string{"dict", "測"}},
		{[]string{"dict", "測", "expired"}, []string{}},
	}

	c := New(NewMockStorage())

	for _, tst := range tests {
		c.Del(tst.keys)
		got := c.Keys("*")
		sort.Strings(got)
		sort.Strings(tst.want)

		if diff := deep.Equal(got, tst.want); diff != nil {
			t.Errorf("Del(%v): %s\n\ngot:%v\n\nwant:%v", tst.keys, diff, got, tst.want)
		}
	}
}

func TestCore_DGet(t *testing.T) {
	tests := []struct {
		key, field string
		err        error
		want       string
	}{
		{"bytes", "", ErrWrongType, ""},
		{"404", "", ErrNotFound, ""},
		{"expired", "", ErrNotFound, ""},
		{"dict", "404", ErrNotFound, ""},
		{"dict", "banana", nil, "mama"},
		{"dict", "測試", nil, "別れ、比類のない"},
	}

	c := New(NewMockStorage())

	for _, tst := range tests {
		got, err := c.DGet(tst.key, tst.field)
		if err != tst.err {
			t.Errorf("DGet(%q, %q) err: %q != %q", tst.key, tst.field, err, tst.err)
		}
		if string(got) != tst.want {
			t.Errorf("DGet(%q, %q) got: %q != %q", tst.key, tst.field, string(got), tst.want)
		}
	}
}

func TestCore_DKeys(t *testing.T) {
	tests := []struct {
		key  string
		err  error
		want []string
	}{
		{"bytes", ErrWrongType, nil},
		{"expired", nil, nil},
		{"404", nil, nil},
		{"dict", nil, []string{"banana", "測試"}},
	}

	c := New(NewMockStorage())

	for _, tst := range tests {
		got, err := c.DKeys(tst.key)
		sort.Strings(got)
		sort.Strings(tst.want)

		if err != tst.err {
			t.Errorf("DKeys(%q) err: %q != %q", tst.key, err, tst.err)
		}
		if diff := deep.Equal(got, tst.want); diff != nil {
			t.Errorf("DKeys(%q): %s\n\ngot:%v\n\nwant:%v", tst.key, diff, got, tst.want)
		}
	}
}

func TestCore_DSet(t *testing.T) {
	tests := []struct {
		key, field, value string
		err               error
		count             int
	}{
		{"bytes", "", "", ErrWrongType, 0},
		{"404", "共", "共産主義の幽霊", nil, 1},
		{"expired", "not expired", "not expired", nil, 1},
		{"dict", "共", "共産主義の幽霊", nil, 1},
		{"dict", "banana", "mango", nil, 0},
	}

	c := New(NewMockStorage())

	for _, tst := range tests {
		count, err := c.DSet(tst.key, tst.field, []byte(tst.value))
		got, getErr := c.DGet(tst.key, tst.field)
		if err != tst.err {
			t.Errorf("DSet(%q, %q) err: %q != %q", tst.key, tst.field, err, tst.err)
		}
		if err == nil && err != nil {
			t.Errorf("DSet(%q, %q) getErr: %q ", tst.key, tst.field, getErr)
		}
		if err == nil && count != tst.count {
			t.Errorf("DSet(%q, %q) count: %d != %d", tst.key, tst.field, count, tst.count)
		}
		if err == nil && string(got) != tst.value {
			t.Errorf("DSet(%q, %q) got: %q != %q", tst.key, tst.field, string(got), tst.value)
		}
	}
}

func TestCore_DGetAll(t *testing.T) {
	tests := []struct {
		key  string
		want map[string]string
		err  error
	}{
		{"bytes", nil, ErrWrongType},
		{"404", map[string]string{}, nil},
		{"expired", map[string]string{}, nil},
		{"dict", map[string]string{"banana": "mama", "測試": "別れ、比類のない"}, nil},
	}

	c := New(NewMockStorage())

	for _, tst := range tests {
		result, err := c.DGetAll(tst.key)
		if err != tst.err {
			t.Errorf("DGet(%q) err: %q != %q", tst.key, err, tst.err)
		}
		got := map[string]string{}
		for i, v := range result {
			if i%2 == 1 {
				// skip values
				continue
			}
			got[string(v)] = string(result[i+1])
		}
		if diff := deep.Equal(got, tst.want); err == nil && diff != nil {
			t.Errorf("DGetAll(%q): %s\n\ngot:%v\n\nwant:%v", tst.key, diff, got, tst.want)
		}
	}
}

func TestCore_DDel(t *testing.T) {
	tests := []struct {
		key       string
		fields    []string
		err       error
		wantKeys  []string
		wantCount int
	}{
		{"bytes", nil, ErrWrongType, nil, 0},
		{"404", []string{"banana", "nothing"}, nil, nil, 0},
		{"expired", []string{"banana", "nothing"}, nil, nil, 0},
		{"dict", []string{"banana", "nothing"}, nil, []string{"測試"}, 1},
	}

	c := New(NewMockStorage())

	for _, tst := range tests {
		count, err := c.DDel(tst.key, tst.fields)
		got, _ := c.DKeys(tst.key)
		sort.Strings(got)
		sort.Strings(tst.wantKeys)

		if err != tst.err {
			t.Errorf("DDel(%q, %q) err: %q != %q", tst.key, tst.fields, err, tst.err)
		}
		if count != tst.wantCount {
			t.Errorf("DDel(%q, %q) count: %d != %d", tst.key, tst.fields, count, tst.wantCount)
		}
		if diff := deep.Equal(got, tst.wantKeys); diff != nil {
			t.Errorf("DKeys(%q, %q): %s\n\ngot:%v\n\nwant:%v", tst.key, tst.fields, diff, got, tst.wantKeys)
		}
	}
}

func TestCore_LLen(t *testing.T) {
	tests := []struct {
		key  string
		err  error
		want int
	}{
		{"bytes", ErrWrongType, 0},
		{"404", nil, 0},
		{"expired", nil, 0},
		{"list", nil, 3},
	}

	c := New(NewMockStorage())

	for _, tst := range tests {
		got, err := c.LLen(tst.key)

		if err != tst.err {
			t.Errorf("LLen(%q) err: %q != %q", tst.key, err, tst.err)
		}
		if got != tst.want {
			t.Errorf("LLen(%q) count: %d != %d", tst.key, got, tst.want)
		}
	}
}

func TestCore_LRange(t *testing.T) {
	tests := []struct {
		key         string
		start, stop int
		err         error
		want        []string
	}{
		{"bytes", 0, 0, ErrWrongType, []string{}},
		// IMPORTANT: in Redis Lrange both on not existing list, or with start/stop out of range returns empty list, not <nil> aka NotFound!
		{"404", 0, 0, nil, []string{}},
		{"expired", 0, 0, nil, []string{}},
		// IMPORTANT: by proto, HEAD of the list has index 0
		{"list", 0, 0, nil, []string{"KMFDM"}},
		{"list", 0, 10, nil, []string{"KMFDM", "Rammstein", "Abba"}},
		{"list", 1, 2, nil, []string{"Rammstein", "Abba"}},
		{"list", 10, 10, nil, []string{}},
		{"list", -2, -1, nil, []string{"Rammstein", "Abba"}},
		{"list", -1, 10, nil, []string{"Abba"}},
		{"list", -3, -3, nil, []string{"KMFDM"}},
		{"list", -1, -2, nil, []string{}},
		{"list", -10, -10, nil, []string{}},
		{"list", -1, -1, nil, []string{"Abba"}},
	}

	c := New(NewMockStorage())

	for _, tst := range tests {
		result, err := c.LRange(tst.key, tst.start, tst.stop)
		got := make([]string, len(result))
		for i, b := range result {
			got[i] = string(b)
		}

		if err != tst.err {
			t.Errorf("LRange(%q, %d, %d) err: %q != %q", tst.key, tst.start, tst.stop, err, tst.err)
		}
		if diff := deep.Equal(got, tst.want); diff != nil {
			t.Errorf("LRange(%q, %d, %d): %s\n\ngot:%v\n\nwant:%v", tst.key, tst.start, tst.stop, diff, got, tst.want)
		}
	}
}

func TestCore_LIndex(t *testing.T) {
	tests := []struct {
		key   string
		index int
		err   error
		want  string
	}{
		{"bytes", 0, ErrWrongType, ""},
		{"404", 0, ErrNotFound, ""},
		{"expired", 0, ErrNotFound, ""},
		//IMPORTANT: by proto, HEAD of the list has index 0
		{"list", 0, nil, "KMFDM"},
		{"list", 10, ErrNotFound, ""},
		{"list", 2, nil, "Abba"},
		{"list", -1, nil, "Abba"},
		{"list", -3, nil, "KMFDM"},
		{"list", -10, ErrNotFound, ""},
	}

	c := New(NewMockStorage())

	for _, tst := range tests {
		result, err := c.LIndex(tst.key, tst.index)
		got := string(result)

		if err != tst.err {
			t.Errorf("LIndex(%q, %d) err: %q != %q", tst.key, tst.index, err, tst.err)
		}
		if got != tst.want {
			t.Errorf("LIndex(%q, %d) got: %q != %q", tst.key, tst.index, got, tst.want)
		}
	}
}

func TestCore_LSet(t *testing.T) {
	tests := []struct {
		key   string
		index int
		err   error
		value string
	}{
		{"bytes", 0, ErrWrongType, ""},
		{"404", 0, ErrNoSuchKey, ""},
		{"expired", 0, ErrNoSuchKey, ""},
		//IMPORTANT: by proto, HEAD of the list has index 0
		{"list", 10, ErrInvalidIndex, ""},
		{"list", 0, nil, "AC/DC"},
		{"list", -1, nil, "Оргия праведников"},
		{"list", -10, ErrInvalidIndex, ""},
	}

	c := New(NewMockStorage())

	for _, tst := range tests {
		err := c.LSet(tst.key, tst.index, []byte(tst.value))
		result, _ := c.LIndex(tst.key, tst.index)
		got := string(result)

		if err != tst.err {
			t.Errorf("LSet(%q, %d, %q) err: %q != %q", tst.key, tst.index, tst.value, err, tst.err)
		}
		if err == nil && got != tst.value {
			t.Errorf("LSet(%q, %d, %q) got: %q != %q", tst.key, tst.index, tst.value, got, tst.value)
		}
	}
}

func TestCore_LPush(t *testing.T) {
	tests := []struct {
		key          string
		err          error
		values, want []string
	}{
		{"bytes", ErrWrongType, nil, nil},
		{"404", nil, []string{"a", "b", "c"}, []string{"c", "b", "a"}},
		{"expired", nil, []string{"a", "b", "c"}, []string{"c", "b", "a"}},
		{"list", nil, []string{"a", "b", "c", "d", "e", "AC/DC"}, []string{"AC/DC", "e", "d", "c", "b", "a", "KMFDM", "Rammstein", "Abba"}},
	}

	c := New(NewMockStorage())

	for _, tst := range tests {
		values := make([][]byte, len(tst.values))
		for i, value := range tst.values {
			values[i] = []byte(value)
		}

		count, err := c.LPush(tst.key, values)
		result, _ := c.LRange(tst.key, 0, -1)

		got := make([]string, len(result))
		for i, value := range result {
			got[i] = string(value)
		}

		if err != tst.err {
			t.Errorf("LPush(%q, %q) err: %q != %q", tst.key, tst.values, err, tst.err)
		}
		if err == nil && count != len(tst.want) {
			t.Errorf("LPush(%q, %q) count: %d != %d", tst.key, tst.values, count, len(tst.want))
		}
		if diff := deep.Equal(got, tst.want); err == nil && diff != nil {
			t.Errorf("LPush(%q, %q): %s\n\ngot:%v\n\nwant:%v", tst.key, tst.values, diff, got, tst.want)
		}
	}
}

func TestCore_LPop(t *testing.T) {
	tests := []struct {
		key        string
		err        error
		wantResult string
		wantList   []string
	}{
		{"bytes", ErrWrongType, "", nil},
		{"404", ErrNotFound, "", []string{}},
		{"expired", ErrNotFound, "", []string{}},
		{"list", nil, "KMFDM", []string{"Rammstein", "Abba"}},
		{"list", nil, "Rammstein", []string{"Abba"}},
		{"list", nil, "Abba", []string{}},
		{"list", ErrNotFound, "", []string{}},
	}

	c := New(NewMockStorage())

	for _, tst := range tests {
		value, err := c.LPop(tst.key)
		result, _ := c.LRange(tst.key, 0, -1)

		got := make([]string, len(result))
		for i, value := range result {
			got[i] = string(value)
		}

		if err != tst.err {
			t.Errorf("LPop(%q) err: %q != %q", tst.key, err, tst.err)
		}
		if err == nil && string(value) != tst.wantResult {
			t.Errorf("LPop(%q) value: %q != %q", tst.key, string(value), tst.wantResult)
		}
		if diff := deep.Equal(got, tst.wantList); err == nil && diff != nil {
			t.Errorf("LPop(%q): %s\n\ngot:%v\n\nwant:%v", tst.key, diff, got, tst.wantList)
		}
	}
}

type TestCoreConcurrencyTestCase struct {
	bytes      []string
	list       []string
	dict       []string
	dictFields []string
	listLen    int
}

func TestCore_concurrency(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}

	tests := []TestCoreConcurrencyTestCase{
		{
			[]string{"b_a", "b_b", "b_c"},
			[]string{"l_a", "l_b", "l_c"},
			[]string{"d_a", "d_b", "d_c"},
			[]string{"f1", "f2", "f3", "f4"},
			10,
		},
		{
			[]string{"b_1", "b_2", "b_3"},
			[]string{"l_1", "l_2", "l_3"},
			[]string{"d_1", "d_2", "d_3"},
			[]string{"f1", "f2", "f3", "f4"},
			10,
		},
		{
			[]string{"b_a", "b_b", "b_c", "b_d", "b_e"},
			[]string{"l_a", "l_b", "l_c", "l_d", "l_e"},
			[]string{"d_a", "d_b", "d_c", "d_d", "d_e"},
			[]string{"f1", "f2", "f3", "f4"},
			10,
		},
	}

	var longTest TestCoreConcurrencyTestCase
	longTest.dictFields = []string{"f1", "f2", "f3", "f4"}
	longTest.listLen = 10
	for i := 0; i < 1000; i++ {
		longTest.bytes = append(longTest.bytes, fmt.Sprintf("b_%d", rand.Uint64()))
		longTest.list = append(longTest.list, fmt.Sprintf("l_%d", rand.Uint64()))
		longTest.dict = append(longTest.dict, fmt.Sprintf("d_%d", rand.Uint64()))
	}
	tests = append(tests, longTest)

	c := New(NewStorageHash())

	stopCollector := make(chan struct{})
	go coreCollectWorker(c, stopCollector)

	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go coreConcurrencyWorker(&wg, c, tests)
	}

	wg.Wait()
	stopCollector <- struct{}{}

	// Due to last operation of every coreConcurrencyWorker is AddOrReplaceOne() for last keyset
	// after all workers done, only last keyset  should remain in the storage
	got := c.Keys("*")
	want := append([]string{}, tests[0].bytes...)
	want = append(want, tests[0].list...)
	want = append(want, tests[0].dict...)
	sort.Strings(got)
	sort.Strings(want)
	if diff := deep.Equal(got, want); diff != nil {
		t.Errorf("Keys() got != Keys() want: %s\n\ngot:%v\n\nwant:%v", diff, got, want)
	}
}

// Tests all Core operations to be concurrent-safe
func coreConcurrencyWorker(wg *sync.WaitGroup, c *Core, tests []TestCoreConcurrencyTestCase) {
	for _, t := range tests {
		for _, key := range t.bytes {
			c.Set(key, []byte(time.Now().String()))
			c.Get(key)

			c.SetEx(key, 1000, []byte(time.Now().String()))
			c.Persist(key)
			c.Expire(key, 1000)
			c.Ttl(key)
		}
		for _, key := range t.dict {
			for _, field := range t.dictFields {
				c.DSet(key, field, []byte(time.Now().String()))
				c.DGet(key, field)
			}
			c.DKeys(key)
			c.DGetAll(key)
			c.DDel(key, t.dictFields)
		}
		for _, key := range t.list {
			var values [][]byte
			for i := 0; i < t.listLen; i++ {
				values = append(values, []byte(time.Now().String()))
			}
			c.LPush(key, values)
			for i := 0; i < t.listLen; i++ {
				c.LSet(key, i, []byte(time.Now().String()))
				c.LIndex(key, i)
			}
			c.LLen(key)
			c.LRange(key, 0, -1)
			for i := 0; i < t.listLen; i++ {
				c.LPop(key)
			}
		}

		c.Keys("**")
		c.Del(t.bytes)
		c.Del(t.list)
		c.Del(t.dict)
	}

	// add first test to check that data actually adds to storage
	t := tests[0]
	for _, key := range t.bytes {
		c.Set(key, []byte(time.Now().String()))
	}
	for _, key := range t.dict {
		c.DSet(key, "f", []byte(time.Now().String()))
	}
	for _, key := range t.list {
		c.LPush(key, [][]byte{[]byte("val")})
	}

	wg.Done()
}

func TestCore_CollectExpired(t *testing.T) {
	collectExpiredTestRunner(t, persistWorker)
	collectExpiredTestRunner(t, expireLaterWorker)
	collectExpiredTestRunner(t, setWorker)
}

func collectExpiredTestRunner(
	t *testing.T,
	worker func(wg *sync.WaitGroup, core *Core, keys, persisted, failed chan string),
) {
	// Initialize
	keysCount := 10000
	maxTtl := 50
	persistWorkersCount := 100
	keysChan := make(chan string, keysCount)
	persistedChan := make(chan string, keysCount)
	failedChan := make(chan string, keysCount)
	data := map[string]*Item{}
	for i := 0; i < keysCount; i++ {
		key := fmt.Sprintf("b_%d", rand.Uint64())
		item := NewItemBytes([]byte("item: " + key))
		item.SetMilliTtl(1 + int(rand.Uint32()%uint32(maxTtl)))

		keysChan <- key
		data[key] = item
	}
	close(keysChan)

	expirationTimer := time.After(time.Duration(maxTtl) * time.Millisecond)

	e := NewStorageHash()
	e.SetData(data)
	c := New(e)
	stopCollector := make(chan struct{})
	wg := sync.WaitGroup{}

	// Run workers
	go coreCollectWorker(c, stopCollector)
	for i := 0; i < persistWorkersCount; i++ {
		wg.Add(1)
		go worker(&wg, c, keysChan, persistedChan, failedChan)
	}

	wg.Wait()
	close(persistedChan)
	close(failedChan)

	// ensure, that all volatile items expired
	<-expirationTimer
	stopCollector <- struct{}{}
	<-stopCollector

	// Collect results
	var persistedKeys, failedKeys []string
	for k := range persistedChan {
		persistedKeys = append(persistedKeys, k)
	}
	for k := range failedChan {
		failedKeys = append(failedKeys, k)
	}

	//t.Logf("Persisted: %d, Failed to persist: %d\n", len(persistedKeys), len(failedKeys))

	// Check results
	var actualKeys []string
	for k, v := range e.Data() {
		if v.IsExpired() {
			t.Errorf("Expired key in result DB: %q : %q", k, v)
			continue
		}

		actualKeys = append(actualKeys, k)
	}

	sort.Strings(actualKeys)
	sort.Strings(persistedKeys)

	if diff := deep.Equal(actualKeys, persistedKeys); diff != nil {
		t.Errorf("actual key not equal persisted keys: %s\n\ngot:%v\n\nwant:%v", diff, actualKeys, persistedKeys)
	}
}

func coreCollectWorker(core *Core, stopCollector chan struct{}) {
	var iterations, totalCount int
	for {
		select {
		case <-stopCollector:
			//to ensure one more than 1 interation done
			totalCount += core.CollectExpired()
			iterations++
			close(stopCollector)
			//fmt.Printf("CollectExpired iteration: %d, total items removed: %d\n", iterations, totalCount)
			return
		default:
			totalCount += core.CollectExpired()
			iterations++
		}
	}
}

func persistWorker(wg *sync.WaitGroup, core *Core, keys, persisted, failed chan string) {
	for key := range keys {
		if core.Persist(key) == 1 {
			persisted <- key
		} else {
			failed <- key
		}
	}
	wg.Done()
}

func expireLaterWorker(wg *sync.WaitGroup, core *Core, keys, persisted, failed chan string) {
	for key := range keys {
		if core.Expire(key, 10000) == 1 {
			persisted <- key
		} else {
			failed <- key
		}
	}
	wg.Done()
}

func setWorker(wg *sync.WaitGroup, core *Core, keys, persisted, failed chan string) {
	_ = failed
	for key := range keys {
		core.Set(key, []byte("data"))
		persisted <- key
	}
	wg.Done()
}

func TestCore_SetEx(t *testing.T) {
	tests := []struct {
		key       string
		value     string
		ttl       int
		wantValue string
	}{
		{"bytes", "Ктулху фхтагн!", 10, "Ктулху фхтагн!"},
		{"dict", "dict", 0, ""},
		{"new 測", "共産主義の幽霊", 11, "共産主義の幽霊"},
		{"expired", "not expired", 12, "not expired"},
	}

	storage := NewMockStorage()
	c := New(storage)

	for _, tst := range tests {
		c.SetEx(tst.key, tst.ttl, []byte(tst.value))
		got, _ := c.Get(tst.key)
		if string(got) != tst.wantValue {
			t.Errorf("SetEx(%q) got: %q != %q", tst.key, string(got), tst.value)
		}
		if got != nil && storage.data[tst.key].Ttl() != tst.ttl {
			t.Errorf("SetEx(%q) ttl: %d != %d, %q", tst.key, storage.data[tst.key].Ttl(), tst.ttl, storage.data[tst.key])
		}
	}
}
func TestCore_Persist(t *testing.T) {
	tests := []struct {
		key        string
		wantResult int
	}{
		{"bytes", 1},
		{"dict", 0},
		{"404", 0},
		{"expired", 0},
	}

	storage := NewMockStorage()
	c := New(storage)

	for _, tst := range tests {
		result := c.Persist(tst.key)
		if result != tst.wantResult {
			t.Errorf("Persist(%q) result: %q != %q", tst.key, result, tst.wantResult)
		}
		if result == 1 && storage.data[tst.key].HasTtl() {
			t.Errorf("Persist(%q): item still volatile", tst.key)
		}
	}
}
func TestCore_Expire(t *testing.T) {
	tests := []struct {
		key        string
		ttl        int
		wantResult int
		wantExists bool
	}{
		{"bytes", 10, 1, true},
		{"dict", 0, 1, false},
		{"404", 11, 0, false},
		{"expired", 12, 0, false},
	}

	storage := NewMockStorage()
	c := New(storage)

	for _, tst := range tests {
		result := c.Expire(tst.key, tst.ttl)
		if result != tst.wantResult {
			t.Errorf("Expire(%q) result: %q != %q", tst.key, result, tst.wantResult)
		}
		if got, _ := c.Get(tst.key); tst.wantExists != (got != nil) {
			t.Errorf("Expire(%q) existanse: %t != %t", tst.key, got != nil, tst.wantExists)
		}
		if tst.wantExists && storage.data[tst.key].Ttl() != tst.ttl {
			t.Errorf("Expire(%q) ttl: %d != %d", tst.key, storage.data[tst.key].Ttl(), tst.ttl)
		}
	}
}
func TestCore_Ttl(t *testing.T) {
	tests := []struct {
		key     string
		wantTtl int
		wantErr error
	}{
		{"bytes", 1000, nil},
		{"dict", -1, nil},
		{"404", -2, nil},
		{"expired", -2, nil},
	}

	c := New(NewMockStorage())

	for _, tst := range tests {
		ttl, err := c.Ttl(tst.key)
		if err != tst.wantErr {
			t.Errorf("Ttl(%q) err: %q != %q", tst.key, err, tst.wantErr)
		}
		if ttl != tst.wantTtl {
			t.Errorf("Ttl(%q) ttl: %d != %d", tst.key, ttl, tst.wantTtl)
		}
	}
}
