package x11

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/jezek/xgb/xproto"
)

func (k *Key) MarshalYAML() (interface{}, error) {
	return fmt.Sprintf("%d;%d", k.Mod, k.Code), nil
}

func (k *Key) UnmarshalYAML(unmarshal func(interface{}) error) error {
	var keystr string

	if err := unmarshal(&keystr); err != nil {
		return err
	}

	splits := strings.Split(keystr, ";")
	if len(splits) != 2 {
		return fmt.Errorf("invalid split count")
	}

	mod, err := strconv.Atoi(splits[0])
	if err != nil {
		return err
	}
	k.Mod = Keymod(mod)

	code, err := strconv.Atoi(splits[1])
	if err != nil {
		return err
	}
	k.Code = xproto.Keycode(code)

	return nil
}
