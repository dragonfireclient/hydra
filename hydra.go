package main

import (
	_ "embed"
	"github.com/dragonfireclient/hydra-dragonfire/convert"
	"github.com/yuin/gopher-lua"
	"os"
	"os/signal"
	"syscall"
	"time"
)

var lastTime = time.Now()
var canceled = false

var serializeVer uint8 = 28
var protoVer uint16 = 39

//go:embed builtin/luax/init.lua
var builtinLuaX string

//go:embed builtin/vector.lua
var builtinVector string

//go:embed builtin/escapes.lua
var builtinEscapes string

//go:embed builtin/client.lua
var builtinClient string

var builtinFiles = []string{
	builtinLuaX,
	builtinVector,
	builtinEscapes,
	builtinClient,
}

var hydraFuncs = map[string]lua.LGFunction{
	"client":     l_client,
	"dtime":      l_dtime,
	"canceled":   l_canceled,
	"poll":       l_poll,
	"disconnect": l_disconnect,
}

func signalChannel() chan os.Signal {
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt, syscall.SIGTERM, syscall.SIGHUP)
	return sig
}

func l_dtime(l *lua.LState) int {
	l.Push(lua.LNumber(time.Since(lastTime).Seconds()))
	return 1
}

func l_canceled(l *lua.LState) int {
	l.Push(lua.LBool(canceled))
	return 1
}

func l_poll(l *lua.LState) int {
	client, pkt, timeout := doPoll(l, getClients(l))
	if client == nil {
		l.Push(lua.LNil)
	} else {
		l.Push(client.userdata)
	}
	l.Push(convert.PushPkt(l, pkt))
	l.Push(lua.LBool(timeout))
	return 3
}

func l_disconnect(l *lua.LState) int {
	for _, client := range getClients(l) {
		client.disconnect()
	}

	return 0
}

func main() {
	if len(os.Args) < 2 {
		panic("missing filename")
	}

	go func() {
		<-signalChannel()
		canceled = true
	}()

	l := lua.NewState(lua.Options{IncludeGoStackTrace: true})
	defer l.Close()

	arg := l.NewTable()
	for i, a := range os.Args {
		l.RawSetInt(arg, i-1, lua.LString(a))
	}
	l.SetGlobal("arg", arg)

	hydra := l.SetFuncs(l.NewTable(), hydraFuncs)
	l.SetField(hydra, "BS", lua.LNumber(10.0))
	l.SetField(hydra, "serialize_ver", lua.LNumber(serializeVer))
	l.SetField(hydra, "proto_ver", lua.LNumber(protoVer))
	l.SetGlobal("hydra", hydra)

	l.SetField(l.NewTypeMetatable("hydra.auth"), "__index", l.SetFuncs(l.NewTable(), authFuncs))
	l.SetField(l.NewTypeMetatable("hydra.client"), "__index", l.NewFunction(l_client_index))

	for _, str := range builtinFiles {
		if err := l.DoString(str); err != nil {
			panic(err)
		}
	}

	if err := l.DoFile(os.Args[1]); err != nil {
		panic(err)
	}
}
