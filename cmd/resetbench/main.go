package main

import (
	"context"
	"errors"
	"fmt"
	"math"
	"sync"
	"time"

	"github.com/jezek/xgb"
	"github.com/jezek/xgb/xproto"
	"github.com/woofdoggo/resetti/internal/mc"
	"github.com/woofdoggo/resetti/internal/reset"
	"github.com/woofdoggo/resetti/internal/x11"
)

func main() {
	x, err := x11.NewClient()
	if err != nil {
		fmt.Println("Failed to connect to X:", err)
		return
	}
	timeOffset, err := approximateTime()
	if err != nil {
		fmt.Println("Failed to get time offset:", err)
		return
	}

	instances, err := mc.FindInstances(x)
	if err != nil {
		fmt.Println("Failed to find instances:", err)
		return
	}
	wg := sync.WaitGroup{}
	readers := make([]reset.LogReader, 0)
	lastStates := make([]mc.InstanceState, 0)
	for _, info := range instances {
		reader, err := reset.NewLogReader(context.Background(), &wg, info)
		if err != nil {
			fmt.Println("Failed to start log reader:", err)
			return
		}
		readers = append(readers, reader)
		lastStates = append(lastStates, mc.InstanceState{})
	}
	ch, errch := reset.Mux(readers)

	for _, instance := range instances {
		timestamp := xproto.Timestamp(time.Now().UnixMilli() - timeOffset)
		if err = x.Click(instance.Wid); err != nil {
			fmt.Println("Failed click:", err)
			return
		}
		time.Sleep(100 * time.Millisecond)
		x.SendKeyPress(
			instance.ResetKey.Code,
			instance.Wid,
			&timestamp,
		)
	}

	resetCount := 0
	start := time.Now()
	fmt.Println("Start:", start)
	for resetCount != 2000 {
		select {
		case state := <-ch:
			last := lastStates[state.Id].State
			next := state.State.State
			if last != mc.StPreview && next == mc.StPreview {
				timestamp := xproto.Timestamp(time.Now().UnixMilli() - timeOffset)
				x.SendKeyPress(
					instances[state.Id].ResetKey.Code,
					instances[state.Id].Wid,
					&timestamp,
				)
				resetCount += 1
				fmt.Printf("Reset %d: %v\n", resetCount, time.Since(start))
			}
			lastStates[state.Id] = state.State
		case err := <-errch:
			fmt.Println("Error:", err)
			return
		}
	}
}

func approximateTime() (int64, error) {
	x, err := xgb.NewConn()
	if err != nil {
		return 0, err
	}
	defer x.Close()
	err = xproto.ChangeWindowAttributesChecked(
		x,
		xproto.Setup(x).DefaultScreen(x).Root,
		xproto.CwEventMask,
		[]uint32{xproto.EventMaskPropertyChange | xproto.EventMaskSubstructureNotify},
	).Check()
	if err != nil {
		return 0, err
	}
	res, err := xproto.InternAtom(x, false, uint16(len("WM_NAME")), "WM_NAME").Reply()
	if err != nil {
		return 0, err
	}
	atom := res.Atom

	offsets := []int64{}
	for i := 0; i < 10; i += 1 {
		send := time.Now().UnixMilli()
		xproto.ChangeProperty(
			x,
			xproto.PropModeAppend,
			xproto.Setup(x).DefaultScreen(x).Root,
			atom,
			xproto.AtomString,
			8,
			0,
			[]byte{},
		)
		rawEvt, err := x.WaitForEvent()
		if rawEvt == nil && err == nil {
			return 0, errors.New("connection died")
		} else if err != nil {
			return 0, err
		}
		evt, ok := rawEvt.(xproto.PropertyNotifyEvent)
		if !ok {
			return 0, fmt.Errorf("invalid event type (%T)", rawEvt)
		}
		diff := send - int64(evt.Time)
		offsets = append(offsets, diff)
	}

	avg := uint64(0)
	max := uint64(0)
	min := uint64(math.MaxUint64)
	for _, offset := range offsets {
		avg += uint64(offset)
		if uint64(offset) > max {
			max = uint64(offset)
		} else if uint64(offset) < min {
			min = uint64(offset)
		}
	}
	avg /= 10
	return int64(avg), nil
}
