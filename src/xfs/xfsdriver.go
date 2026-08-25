package xfsdriver

import "sync"

type XFSDriver struct {
	M *sync.Mutex
}
