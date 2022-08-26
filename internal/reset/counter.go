package reset

import (
	"log"
	"os"
	"strconv"
	"strings"

	"github.com/woofdoggo/resetti/internal/cfg"
)

func incrementResets(f *os.File, count int, countCh chan<- int) {
	countCh <- count
	if f == nil {
		return
	}
	_, err := f.Seek(0, 0)
	if err != nil {
		log.Printf("incrementResets: seek err: %s", err)
		return
	}
	_, err = f.WriteString(strconv.Itoa(count))
	if err != nil {
		log.Printf("incrementResets: write err: %s", err)
		return
	}
}

func openCounter(conf cfg.Profile) (*os.File, int, error) {
	if !conf.General.CountResets {
		return nil, 0, nil
	}
	file, err := os.OpenFile(conf.General.CountPath, os.O_RDWR|os.O_CREATE, 0644)
	if err != nil {
		return nil, 0, err
	}
	buf := make([]byte, 16)
	n, err := file.Read(buf)
	if err != nil {
		file.Close()
		return nil, 0, err
	}
	if n == 0 {
		return file, 0, nil
	}
	c, err := strconv.Atoi(strings.Trim(string(buf[:n]), "\n"))
	if err != nil {
		file.Close()
		return nil, 0, err
	}
	return file, c, nil
}
