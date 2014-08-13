package golog

import (
        "fmt"
        "os"
        "runtime"
        "sync"
        "sync/atomic"
        "syscall"
        "time"
)

const (
        LOG_MAX_SIZE = 1024 * 1024 * 8
        LOG_NONE     = 0x00
        LOG_CRIT     = 0x1000
        LOG_ERR      = 0x2000
        LOG_DEBUG    = 0x3000
        LOG_INFO     = 0x4000
)

type Hlog struct {
        prefix        string
        v             chan string
        log_max_size  int64
        w             *os.File
        w_sync        *sync.WaitGroup
        log_level_str map[int]string
        level         int
        is_close      bool
}

var g_file_suffix uint32 = 100

//Create server to provide you write log
func NewLog(filename, prefix string, size int64, level int) *Hlog {
        var logger Hlog

        if size <= 0 {
                logger.log_max_size = LOG_MAX_SIZE
        } else {
                logger.log_max_size = size
        }
        if filename == "" {
                logger.w = os.Stdout
        } else {
                oflag := os.O_APPEND | os.O_WRONLY
                if _, err := os.Stat(filename); err != nil {
                        oflag |= os.O_CREATE
                }
                f, err := os.OpenFile(filename, oflag, 0644)
                if err != nil {
                        fmt.Printf("Open %s failed: %v\n", filename, err)
                        return nil
                }
                logger.w = f
        }

        switch level {
        case LOG_CRIT, LOG_ERR, LOG_DEBUG, LOG_INFO:
                logger.level = level
        default:
                fmt.Printf("UNKNOWN log level %d\n", level)
                return nil
        }

        logger.prefix = prefix
        logger.is_close = false
        logger.w_sync = new(sync.WaitGroup)
        logger.v = make(chan string)
        logger.log_level_str = map[int]string{
                LOG_CRIT:  "CRIT",
                LOG_ERR:   "ERR",
                LOG_DEBUG: "DEBUG",
                LOG_INFO:  "INFO",
        }

        go func() {
                //可以检测channel是否已关闭，关闭的话循环结束
                for v := range logger.v {
                        logger.w.Write([]byte(v))
                        if fi, err := logger.w.Stat(); err == nil {
                                if fi.Size() >= logger.log_max_size {
                                        logger.w.Close()
                                        name := logger.w.Name()
                                        t := time.Now()
                                        newName := fmt.Sprintf("%s.%04d%02d%02d%02d%02d%02d_%d", name,
                                                t.Year(), t.Month(), t.Day(),
                                                t.Hour(), t.Minute(), t.Second(),
                                                atomic.AddUint32(&g_file_suffix, 1))
                                        os.Rename(name, newName)
                                        oflag := os.O_APPEND | os.O_WRONLY
                                        if _, err := os.Stat(filename); err != nil {
                                                oflag |= os.O_CREATE
                                        }
                                        f, err := os.OpenFile(filename, oflag, 0644)
                                        if err != nil {
                                                fmt.Printf("Open %s failed: %v\n", filename, err)
                                                return
                                        }
                                        logger.w = f
                                }
                        }
                }

        }()

        return &logger
}

//Destroy log server
func (h *Hlog) Destroy(level int, format string, v ...interface{}) {
        h.is_close = true
        h.w_sync.Wait()
        close(h.v)
        t := time.Now()
        s := h.prefix + " <" + h.log_level_str[level] + "> " + fmt.Sprintf("%04d/%02d/%02d %02d:%02d:%02d",
                t.Year(), t.Month(), t.Day(), t.Hour(), t.Minute(), t.Second())
        pc, f, line, ok := runtime.Caller(1)
        if ok {
                fn := runtime.FuncForPC(pc)
                s = s + fmt.Sprintf(" [filename:%s, line:%d, function:%v] ",
                        f, line, fn.Name())

        }
        s = s + fmt.Sprintf(format, v...)
        _, err := h.w.Write([]byte(s))
        if err != nil {
                if v, ok := err.(*os.PathError); ok {
                        if v.Err.Error() == syscall.EBADF.Error() {
                                if h.w, err = os.OpenFile(h.w.Name(),
                                        os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644); err != nil {
                                        return
                                }
                                h.w.Write([]byte(s))
                        } else {
                                return
                        }
                } else {
                        return
                }

        }
        h.w.Close()
}

//Write log to stdout or file
func (h *Hlog) Printf(level int, format string, v ...interface{}) {
        if level > h.level {
                return
        }
        h.w_sync.Add(1)
        if !h.is_close {

                t := time.Now()
                s := h.prefix + " <" + h.log_level_str[level] + "> " + fmt.Sprintf("%04d/%02d/%02d %02d:%02d:%02d",
                        t.Year(), t.Month(), t.Day(), t.Hour(), t.Minute(), t.Second())
                pc, f, line, ok := runtime.Caller(1)
                if ok {
                        fn := runtime.FuncForPC(pc)
                        s = s + fmt.Sprintf(" [filename:%s, line:%d, function:%v] ",
                                f, line, fn.Name())

                }
                s = s + fmt.Sprintf(format, v...)

                h.v <- s

        }
        h.w_sync.Done()
}
