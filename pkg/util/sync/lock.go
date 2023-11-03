package sync

import (
	"sync"
)

type NamedLockMap interface {
	LockByName(name string) LockWithData
}

type LockWithData interface {
	Load(key interface{}) (value interface{}, ok bool)
	QLoad(key interface{}) interface{}

	// These automatically do a Lock()/Unlock() when executing
	LoadOrStore(key, value interface{}) (actual interface{}, loaded bool)
	QLoadOrStore(key, value interface{}) interface{}
	Store(key, value interface{})

	sync.Locker

	RLocker() sync.Locker
	RLock()
	RUnlock()

	/*RLock(key string) KeyedLockGetter
	RUnlock(key string)

	Lock(key string) KeyedLockSetter
	Unlock(key string)*/
}

/*type KeyedLockGetter interface {
	Get(key interface{}) interface{}
}

type KeyedLockSetter interface {
	KeyedLockGetter
	Set(key, value interface{})
}*/

func NewNamedLockMap() NamedLockMap {
	return &namedLockMap{
		locks:   make(map[string]*lockWithData),
		locksMu: &sync.Mutex{},
	}
}

type namedLockMap struct {
	// locks maps keys to their individual locks and associated data
	locks map[string]*lockWithData
	// locksMu guards reads and writes of the locks map
	locksMu *sync.Mutex
}

func (l *namedLockMap) LockByName(name string) LockWithData {
	// l.locksMu guards reads and writes of the c.locks map
	l.locksMu.Lock()
	defer l.locksMu.Unlock()

	// Check if information about a transaction on this branch exists.
	txState, ok := l.locks[name]
	if ok {
		return txState
	}
	// if not, grow the txs map by one and return it
	l.locks[name] = &lockWithData{
		RWMutex: &sync.RWMutex{},
		Map:     &sync.Map{},
	}
	return l.locks[name]
}

type lockWithData struct {
	*sync.RWMutex
	*sync.Map
	//data map[interface{}]interface{}
}

func (l *lockWithData) QLoad(key interface{}) interface{} {
	value, _ := l.Map.Load(key)
	return value
}

func (l *lockWithData) QLoadOrStore(key, value interface{}) interface{} {
	actual, _ := l.Map.LoadOrStore(key, value)
	return actual
}

/*
func (l *lockWithData) RLock()   { l.mu.RLock() }
func (l *lockWithData) RUnlock() { l.mu.RUnlock() }
func (l *lockWithData) Lock()    { l.mu.Lock() }
func (l *lockWithData) Unlock()  { l.mu.Unlock() }
*/

/*func (l *lockWithData) Get(key interface{}) interface{} {
	return l.data[key]
}

type writableLockWithData struct {
	*lockWithData
}

func (l *writableLockWithData) Set(key, value interface{}) {
	l.data[key] = value
}*/
