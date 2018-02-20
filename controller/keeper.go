package controller

import (
	"encoding/gob"
	"fmt"
	"github.com/mshaverdo/radish/core"
	"github.com/mshaverdo/radish/log"
	"github.com/mshaverdo/radish/message"
	"io"
	"io/ioutil"
	"os"
	"path"
	"path/filepath"
	"sort"
	"sync"
	"time"
)

type SyncPolicy int

const (
	// SyncNever means newer do walFile.Sync()
	SyncNever SyncPolicy = iota

	// SyncNever means walFile.Sync() every second
	SyncSometimes

	// SyncNever means walFile.Sync() every message write
	SyncAlways
)

// Avoid "Unused Constant" warning
var _ = SyncNever

const (
	walFileName     = "wal_%v.gob"
	storageFileName = "storage.gob"
)

type storageData struct {
	MessageId int64
	Engine    core.Engine
}

type Keeper struct {
	takeSnapshotInterval time.Duration
	syncPolicy           SyncPolicy
	dataDir              string
	core                 Core

	processor *Processor

	mutex      sync.Mutex
	messageId  int64
	walFile    *os.File
	walEncoder *gob.Encoder
	lastSync   time.Time
}

func NewKeeper(core Core, dataDir string, policy SyncPolicy, snapshotInterval time.Duration) *Keeper {
	return &Keeper{
		core:                 core,
		dataDir:              dataDir,
		syncPolicy:           policy,
		takeSnapshotInterval: snapshotInterval,
		processor:            NewProcessor(core),
	}
}

// WriteToWal writes request to WAL
func (k *Keeper) WriteToWal(request *message.Request) error {
	k.mutex.Lock()
	defer k.mutex.Unlock()

	k.messageId++
	request.Id = k.messageId
	err := k.walEncoder.Encode(request)

	if k.syncPolicy == SyncAlways || k.syncPolicy == SyncSometimes && time.Since(k.lastSync) > 1*time.Second {
		k.walFile.Sync()
		k.lastSync = time.Now()
	}

	return err
}

// restoreStorageState restores k.core state from dataDir
func (k *Keeper) restoreStorageState() error {
	if err := k.loadStorage(); err != nil {
		return err
	}
	processedWals, err := k.processAllWals()
	if err != nil {
		return err
	}
	// dump storage with merged WALs to disk
	if err := k.persistStorage(); err != nil {
		return err
	}

	// all OK, remove processed WALs
	for _, v := range processedWals {
		err := os.Remove(v)
		if err != nil {
			log.Warningf("Unable to remove processed WAL %s: %s", v, err)
		}
	}

	return nil
}

func (k *Keeper) loadStorage() error {
	filename := path.Join(k.dataDir, storageFileName)
	file, err := os.Open(filename)
	if os.IsNotExist(err) {
		// no data file found, just skip
		return nil
	} else if err != nil {
		return fmt.Errorf("Controller.loadStorage(). Unable to open %s: %s", filename, err)
	}
	defer file.Close()

	log.Infof("Loading storage data from %s...", filename)

	data := storageData{}
	dec := gob.NewDecoder(file)
	if err := dec.Decode(&data); err != nil {
		return fmt.Errorf("Keeper.loadStorage(): Unable to decode stream: %s", err)
	}

	k.core.SetEngine(data.Engine)
	k.messageId = data.MessageId

	if err != nil {
		return err
	}

	return nil
}

func (k *Keeper) processAllWals() (processedWals []string, err error) {
	allFiles, err := filepath.Glob(k.walFileName("*"))
	if err != nil {
		return nil, fmt.Errorf("Keeper.processAllWals(): %s", err)
	}

	var messageIds []int
	for _, v := range allFiles {
		id := 0
		fmt.Sscanf(v, k.walFileName("%d"), &id)
		if id > 0 {
			messageIds = append(messageIds, id)
		}
	}

	sort.Ints(messageIds)

	// process all WALs from earliest to latest
	for _, messageId := range messageIds {
		filename := k.walFileName(messageId)
		if err := k.processWal(filename); err != nil {
			return nil, err
		}
		processedWals = append(processedWals, filename)
	}

	return processedWals, nil
}

func (k *Keeper) processWal(filename string) error {
	log.Infof("processing WAL %s...", filename)

	file, err := os.Open(filename)
	if err != nil {
		return err
	}
	defer file.Close()

	dec := gob.NewDecoder(file)
	req := new(message.Request)
	for err := dec.Decode(req); err != io.EOF; err = dec.Decode(req) {
		if err != nil {
			return fmt.Errorf("Keeper.processWal(): can't process %s: %s", filename, err)
		}

		if req.Id <= k.messageId {
			// skip messages, that already in the storage
			continue
		}

		err = k.processor.FixRequestTtl(req)
		if err != nil {
			return fmt.Errorf("Keeper.processWal(): can't process %s: %s \nrequest: %s", filename, err, req)
		}

		resp := k.processor.Process(req)
		if resp.Status != message.StatusOk {
			// we got an error, but this request was successful. Something went wrong
			return fmt.Errorf("Keeper.processWal(): can't process %s: \nrequest: %s \nresponse: %s", filename, req, resp)
		}

		k.messageId = req.Id
		req = new(message.Request)
	}

	return nil
}

func (k *Keeper) persistStorage() error {
	//remove expired items to decrease dump size
	k.core.CollectExpired()

	file, err := ioutil.TempFile(filepath.Dir(k.storageFileName()), filepath.Base(k.storageFileName()))
	defer file.Close()

	if err != nil {
		return fmt.Errorf("Keeper.persistStorage(): %s", err)
	}

	data := storageData{
		Engine:    k.core.Engine(),
		MessageId: k.messageId,
	}

	enc := gob.NewEncoder(file)
	if err := enc.Encode(data); err != nil {
		return fmt.Errorf("Keeper.persistStorage(): %s", err)
	}

	err = os.Rename(file.Name(), k.storageFileName())
	if err != nil {
		return fmt.Errorf("Keeper.persistStorage(): %s", err)
	}

	return nil
}

// Shutdown shuts Keeper down and persists storage
func (k *Keeper) Shutdown() error {
	if !k.isRunning() {
		panic("Program logic error: Tying to shut down not running Keeper")
	}

	log.Infof("Persisting storage...")
	err := k.persistStorage()
	if err != nil {
		return err
	}

	oldWalFilename := k.walFile.Name()
	k.walFile.Close()
	os.Remove(oldWalFilename)

	return nil
}

// Start restores storage state and starts new WAL
func (k *Keeper) Start() (err error) {
	if k.isRunning() {
		panic("Program logic error: Tying to start already running Keeper")
	}

	err = k.restoreStorageState()
	if err != nil {
		return err
	}

	_, err = k.startNewWal()
	return err
}

// startNewWal closes current WAL file and starts new
func (k *Keeper) startNewWal() (oldWalFilename string, err error) {
	k.mutex.Lock()
	defer k.mutex.Unlock()

	k.messageId++
	filename := k.walFileName(k.messageId)

	if _, err := os.Stat(filename); !os.IsNotExist(err) {
		err = fmt.Errorf("Keeper.startNewWal(): trying to write WAL to existing file: %s", filename)
		log.Error(err.Error())
		return "", err
	}

	file, err := os.Create(filename)
	if err != nil {
		err = fmt.Errorf("Keeper.startNewWal(): error creating WAL file %s: %s", filename, err.Error())
		log.Error(err.Error())
		return "", err
	}

	if k.walFile != nil {
		oldWalFilename = k.walFile.Name()
		k.walFile.Close()
	}

	k.walFile = file
	k.walEncoder = gob.NewEncoder(k.walFile)

	return oldWalFilename, nil
}

func (k *Keeper) walFileName(messageId interface{}) string {
	return path.Join(k.dataDir, fmt.Sprintf(walFileName, messageId))
}

func (k *Keeper) storageFileName() string {
	return path.Join(k.dataDir, storageFileName)
}

func (k *Keeper) isRunning() bool {
	k.mutex.Lock()
	defer k.mutex.Unlock()
	return k.walFile != nil
}
