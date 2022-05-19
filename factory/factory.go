package factory

import (
	"fmt"
	"sync"
	"time"

	"github.com/pojol/gobot/behavior"
	"github.com/pojol/gobot/bot"
	"github.com/pojol/gobot/database"
	"github.com/pojol/gobot/utils"
)

type TaskInfo struct {
	Name string
	Num  int32
}

type Factory struct {
	parm   Parm
	report *Report

	debugBots map[string]*bot.Bot

	pipelineCache []TaskInfo
	batches       []*Batch
	lru           database.LRUCache

	batchLock sync.Mutex

	lock sync.Mutex
	exit *utils.Switch
}

func Create(opts ...Option) (*Factory, error) {

	p := Parm{
		frameRate:   time.Second * 1,
		lifeTime:    time.Minute,
		Interrupt:   true,
		ReportLimit: 100,
		ScriptPath:  "script/",
		batchSize:   2048,
	}

	for _, opt := range opts {
		opt(&p)
	}

	f := &Factory{
		parm:      p,
		report:    NewReport(int32(p.ReportLimit)),
		debugBots: make(map[string]*bot.Bot),
		exit:      utils.NewSwitch(),
		lru:       database.Constructor(100),
	}

	go f.taskLoop()

	Global = f
	return f, nil
}

var Global *Factory

func (f *Factory) GetReport() []database.ReportInfo {
	return f.report.Info()
}

// Close 关闭机器人工厂
func (f *Factory) Close() {
	f.exit.Open()
}

func (f *Factory) AddBehavior(name string, byt []byte) {
	f.lock.Lock()
	defer f.lock.Unlock()

	err := database.Get().UpsetFile(name, byt)
	if err != nil {
		fmt.Println("AddBehavior ", err.Error())
	}

	f.lru.Put(name, byt)
}

func (f *Factory) RmvBehavior(name string) {
	err := database.Get().DelFile(name)
	if err != nil {
		fmt.Println("RmvBehavior ", err.Error())
	}
}

func (f *Factory) UpdateBehaviorTags(name string, tags []byte) []database.BehaviorInfo {
	err := database.Get().UpdateTags(name, tags)
	if err != nil {
		fmt.Println("UpdateBehaviorTags", err.Error())
	}

	return f.GetBehaviors()
}

func (f *Factory) GetBehaviors() []database.BehaviorInfo {
	lst, err := database.Get().GetAllFiles()
	if err != nil {
		fmt.Println("GetBehaviors ", err.Error())
	}

	return lst
}

func (f *Factory) UploadConfig(dat []byte) error {

	err := database.Get().UpsetConfig(dat)
	if err != nil {
		return err
	}

	return nil
}

func (f *Factory) GetConfig() (database.TemplateConfig, error) {
	return database.Get().FindConfig("config")
}

func (f *Factory) FindBehavior(name string) (database.BehaviorInfo, error) {
	return database.Get().FindFile(name)
}

func (f *Factory) AddTask(name string, cnt int32) error {
	_, err := database.Get().FindFile(name)
	if err != nil {
		return err
	}

	f.pipelineCache = append(f.pipelineCache, TaskInfo{Name: name, Num: cnt})
	return nil
}

func (f *Factory) CreateTask(name string, num int) *Batch {

	var dat []byte

	ok, byt := f.lru.Get(name)
	if ok {
		dat = byt.([]byte)

	} else {
		info, err := database.Get().FindFile(name)
		if err != nil {
			return nil
		}

		dat = info.Dat
		f.lru.Put(name, info.Dat)
	}

	return CreateBatch(f.parm.ScriptPath, name, num, dat, int32(f.parm.batchSize), database.GetGlobalScript())
}

func (f *Factory) CreateDebugBot(name string, fbyt []byte) *bot.Bot {
	var b *bot.Bot

	tree, err := behavior.New(fbyt)
	if err != nil {
		return nil
	}

	b = bot.NewWithBehaviorTree(f.parm.ScriptPath, tree, name, 1, database.GetGlobalScript())
	f.debugBots[b.ID()] = b

	return b
}

func (f *Factory) FindBot(botid string) *bot.Bot {

	if _, ok := f.debugBots[botid]; ok {
		return f.debugBots[botid]
	}

	return nil
}

func (f *Factory) RmvBot(botid string) {

	if _, ok := f.debugBots[botid]; ok {
		f.debugBots[botid].Close()
		delete(f.debugBots, botid)
	}

}

func (f *Factory) taskLoop() {
	for {
		f.lock.Lock()
		if len(f.pipelineCache) > 0 {
			info := f.pipelineCache[0]
			f.pipelineCache = f.pipelineCache[1:]

			b := f.CreateTask(info.Name, int(info.Num))

			f.pushBatch(b)
			<-b.BatchDone
			f.popBatch()
		}
		f.lock.Unlock()

		time.Sleep(time.Millisecond)
	}
}

func (f *Factory) pushBatch(b *Batch) {
	fmt.Println("push batch", b.ID, b.Name)

	f.batchLock.Lock()
	f.batches = append(f.batches, b)
	f.batchLock.Unlock()
}

func (f *Factory) popBatch() {

	f.batchLock.Lock()
	b := f.batches[0]
	f.report.Append(b.Report())
	b.Close()

	fmt.Println("pop batch", b.ID, b.Name)

	s := bot.BotStatusUnknow
	if b.Report().ErrNum > 0 {
		s = bot.BotStatusFail
	} else {
		s = bot.BotStatusSucc
	}
	database.Get().UpdateState(b.Name, s)
	f.batches = f.batches[1:]
	f.batchLock.Unlock()
}

func (f *Factory) GetBatchInfo() []BatchInfo {
	var lst []BatchInfo
	f.batchLock.Lock()
	for _, v := range f.batches {
		lst = append(lst, v.Info())
	}
	f.batchLock.Unlock()
	return lst
}

/*
func (f *Factory) router() {


	for {
		select {
		case bot := <-f.translateCh:
			f.push(bot)
			rep.Name = bot.Name()
			bot.Run(f.exit, f.doneCh, f.errCh)
		case id := <-f.doneCh:
			f.pop(id, nil, rep)
		case err := <-f.errCh:
			f.pop(err.ID, err.Err, rep)
		case <-f.batchDone:
			goto ext
		}
	}
ext:
	atomic.AddInt64(&f.IncID, 1)
	// report
	f.Report(rep)

	f.running = false
}
*/
