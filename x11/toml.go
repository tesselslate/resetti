package x11

import (
	"errors"
	"strings"
)

func (m *Keymod) UnmarshalTOML(value interface{}) error {
	str, ok := value.(string)
	if !ok {
		return errors.New("not a string")
	}
	substrs := strings.Split(str, "-")
	for _, s := range substrs {
		if val, ok := mods[strings.ToLower(s)]; ok {
			*m |= val
		}
	}
	return nil
}

func (k *Key) UnmarshalTOML(value interface{}) error {
	str, ok := value.(string)
	if !ok {
		return errors.New("not a string")
	}
	substrs := strings.Split(str, "-")
	for _, s := range substrs {
		if val, ok := keys[strings.ToLower(s)]; ok {
			k.Code = val
		} else if val, ok := mods[strings.ToLower(s)]; ok {
			k.Mod |= val
		}
	}
	return nil
}
