package script

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"net"
	"reflect"
	"sync"
	"syscall"
	"time"

	"github.com/gogo/protobuf/proto"
	"github.com/pojol/gobot/script/pb"
	lua "github.com/yuin/gopher-lua"
	luar "layeh.com/gopher-luar"
)

type TCPModule struct {
	conn *net.TCPConn
	fd   int

	buf   byteQueue
	bufMu sync.Mutex

	writeTime time.Time
	repolst   []Report

	done chan struct{} // 通知协程停止的通道
}

type byteQueue []byte

var byteOrder = binary.BigEndian

// 入队
func (q *byteQueue) push(b []byte) {
	*q = append(*q, b...)
}

// 出队
func (q *byteQueue) pop(maxLen int) ([]byte, bool) {
	if len(*q) == 0 {
		return nil, false
	}

	if maxLen > len(*q) {
		maxLen = len(*q)
	}

	data := (*q)[:maxLen]
	*q = (*q)[maxLen:]
	return data, true
}

// 队列当前长度
func (q *byteQueue) haveFull() bool {
	if len(*q) < 4 {
		return false
	}

	var header [4]byte
	copy(header[:], (*q)[:4])

	var msgleni int32
	binary.Read(bytes.NewBuffer(header[1:]), byteOrder, &msgleni)
	if len(*q) < 4+int(msgleni) {
		return false
	}

	return true
}

func NewTCPModule() *TCPModule {

	tcpm := &TCPModule{}

	return tcpm
}

func (t *TCPModule) Loader(L *lua.LState) int {
	mod := L.SetFuncs(L.NewTable(), map[string]lua.LGFunction{
		"dail":  t.dail,
		"close": t.Close,

		"write": t.write,
		"read":  t.read,

		"read_msg":  t.read_msg,
		"write_msg": t.write_msg,
	})
	L.Push(mod)
	return 1
}

func (t *TCPModule) dail(L *lua.LState) int {
	err := t._dail(L.ToString(1), L.ToString(2))
	if err != nil {
		L.Push(lua.LString(err.Error()))
		return 1
	}

	L.Push(lua.LString("succ"))
	return 1
}

func (t *TCPModule) _dail(host string, port string) error {
	fmt.Println("_dail")
	tcpServer, err := net.ResolveTCPAddr("tcp", host+":"+port)
	if err != nil {
		return fmt.Errorf("resolve tcp addr err:%s", err.Error())
	}

	t.conn, err = net.DialTCP("tcp", nil, tcpServer)
	if err != nil {
		return fmt.Errorf("dial tcp err:%s", err.Error())
	}

	f, _ := t.conn.File()
	t.fd = int(f.Fd())
	t.done = make(chan struct{}, 1)

	go t._read()
	return nil
}

func (t *TCPModule) Close(L *lua.LState) int {

	if t.conn != nil {
		t.conn.Close()
		t.conn = nil
	}

	close(t.done)

	L.Push(lua.LString("succ"))
	return 1
}

func (t *TCPModule) write(L *lua.LState) int {
	if t.conn == nil {
		L.Push(lua.LString("not connected"))
		return 1
	}

	msg := L.ToString(1)
	_, err := t.conn.Write([]byte(msg))
	if err != nil {
		L.Push(lua.LString(err.Error()))
		return 1
	}

	L.Push(lua.LString("succ"))
	return 1
}

func (t *TCPModule) _read() {

	if t.conn == nil {
		return
	}

	for {
		select {
		case <-t.done:
			fmt.Println("chan exit")
			return
		default:
			buf := make([]byte, 1024)
			n, err := t.conn.Read(buf)
			if err != nil {
				fmt.Printf("syscall.read fd %v size %v err %v\n", t.fd, n, err.Error())
				return
			}

			if n != 0 {
				t.bufMu.Lock()
				t.buf.push(buf[:n])
				t.bufMu.Unlock()
			}
		}
	}

}

func readret(L *lua.LState, ty int, id string, body proto.Message, err string) int {

	L.Push(lua.LNumber(ty))
	L.Push(lua.LString(id))
	L.Push(luar.New(L, body))
	L.Push(lua.LString(err))

	return 4
}

func (t *TCPModule) read_msg(L *lua.LState) int {

	msgtyi := int8(0)

	var msgbody []byte

	t.bufMu.Lock()
	if !t.buf.haveFull() {
		t.bufMu.Unlock()
		return readret(L, 0, "", nil, "nodata")
	}

	msgtyb, _ := t.buf.pop(1)

	binary.Read(bytes.NewBuffer(msgtyb), byteOrder, &msgtyi)

	msglenb, _ := t.buf.pop(3)
	msgleni := BytesToInt(msglenb)

	msgbody, _ = t.buf.pop(msgleni)
	t.bufMu.Unlock()

	ccgMsg := pb.TcgMsg{}
	proto.Unmarshal(msgbody, &ccgMsg)

	var body proto.Message
	msgType := proto.MessageType(ccgMsg.LogicType)
	tptr := reflect.New(msgType.Elem())
	body = tptr.Interface().(proto.Message)
	proto.Unmarshal(ccgMsg.LogicData, body)
	info := Report{
		Api:     ccgMsg.LogicType,
		ResBody: int(msgleni),
		Consume: int(time.Since(t.writeTime).Milliseconds()),
	}
	t.repolst = append(t.repolst, info)

	return readret(L, int(msgtyi), ccgMsg.LogicType, body, "")
}

func (t *TCPModule) write_msg(L *lua.LState) int {

	msgid := L.ToString(1)
	msgbody := L.ToString(2)

	if t.conn == nil {
		L.Push(lua.LString("not connected"))
		return 1
	}

	ccg := &pb.TcgMsg{
		LogicType: msgid,
		LogicData: []byte(msgbody),
	}

	data, _ := proto.Marshal(ccg)

	buf := bytes.NewBuffer(make([]byte, 0, 4+len(data)))

	// binary.Write(buf, byteOrder, uint8(4))
	buf.WriteByte(0x04)
	binary.Write(buf, byteOrder, IntToBytes(len(data)))
	buf.WriteString(string(data))

	t.writeTime = time.Now()

	_, err := t.conn.Write(buf.Bytes())
	if err != nil {
		L.Push(lua.LString(err.Error()))
		return 1
	}

	L.Push(lua.LString("succ"))
	return 1
}

func (t *TCPModule) read(L *lua.LState) int {

	if t.conn == nil {
		L.Push(lua.LString("fail"))
		L.Push(lua.LString("not connected"))
		return 2
	}

	buf := make([]byte, 128) //test
	// 非阻塞读取
	n, err := syscall.Read(syscall.Handle(t.fd), buf)
	// 处理读取结果
	if err == syscall.EWOULDBLOCK {
		L.Push(lua.LString("fail"))
		L.Push(lua.LString(err.Error()))
		return 2
	}

	if n == 0 {
		L.Push(lua.LString("fail"))
		L.Push(lua.LString("nodata"))
		return 2
	}

	content := string(buf[:n])
	L.Push(lua.LString("succ"))
	L.Push(lua.LString(content))

	// 立即返回,不阻塞
	return 2
}

func (t *TCPModule) GetReport() []Report {

	rep := []Report{}
	rep = append(rep, t.repolst...)

	t.repolst = t.repolst[:0]

	return rep
}

// BytesToInt decode packet data length byte to int(Big end)
func BytesToInt(b []byte) int {
	result := 0
	for _, v := range b {
		result = result<<8 + int(v)
	}
	return result
}

// IntToBytes encode packet data length to bytes(Big end)
func IntToBytes(n int) []byte {
	buf := make([]byte, 3)
	buf[0] = byte((n >> 16) & 0xFF)
	buf[1] = byte((n >> 8) & 0xFF)
	buf[2] = byte(n & 0xFF)
	return buf
}
