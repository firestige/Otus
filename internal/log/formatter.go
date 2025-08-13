package log

import (
	"fmt"
	"runtime"
	"strings"

	"github.com/sirupsen/logrus"
)

type formatter struct {
	pattern string
	time    string
}

// Format supports unified log output format that has %time, %level, %field, %msg, %caller, %func, %goroutine.
func (f *formatter) Format(entry *logrus.Entry) ([]byte, error) {
	output := f.pattern
	output = strings.Replace(output, "%time", entry.Time.Format(f.time), 1)
	output = strings.Replace(output, "%level", entry.Level.String(), 1)
	output = strings.Replace(output, "%field", buildFields(entry), 1)
	output = strings.Replace(output, "%msg", entry.Message, 1)
	output = strings.Replace(output, "%caller", getCaller(entry), 1)
	output = strings.Replace(output, "%func", getFunc(entry), 1)
	output = strings.Replace(output, "%goroutine", getGoroutineID(), 1)
	return []byte(output), nil
}

// 获取调用位置（package/文件名:行号）
func getCaller(entry *logrus.Entry) string {
	if entry.HasCaller() {
		// 只保留 package 和文件名
		file := entry.Caller.File
		// 去除路径，只保留文件名
		slashIdx := strings.LastIndex(file, "/")
		if slashIdx != -1 && slashIdx+1 < len(file) {
			file = file[slashIdx+1:]
		}
		// 获取包名（从函数名中提取）
		pkg := ""
		if entry.Caller.Function != "" {
			funcParts := strings.Split(entry.Caller.Function, ".")
			if len(funcParts) > 1 {
				pkgParts := strings.Split(funcParts[0], "/")
				pkg = pkgParts[len(pkgParts)-1]
			}
		}
		return fmt.Sprintf("%s/%s:%d", pkg, file, entry.Caller.Line)
	}
	// fallback: 使用runtime
	_, file, line, ok := runtime.Caller(8)
	if ok {
		slashIdx := strings.LastIndex(file, "/")
		if slashIdx != -1 && slashIdx+1 < len(file) {
			file = file[slashIdx+1:]
		}
		return fmt.Sprintf("unknown/%s:%d", file, line)
	}
	return "unknown"
}

// 获取调用函数名（只保留方法或函数名）
func getFunc(entry *logrus.Entry) string {
	if entry.HasCaller() {
		funcName := entry.Caller.Function
		// 只保留最后一个点后的部分
		dotIdx := strings.LastIndex(funcName, ".")
		if dotIdx != -1 && dotIdx+1 < len(funcName) {
			return funcName[dotIdx+1:]
		}
		return funcName
	}
	pc, _, _, ok := runtime.Caller(8)
	if ok {
		fn := runtime.FuncForPC(pc)
		if fn != nil {
			funcName := fn.Name()
			dotIdx := strings.LastIndex(funcName, ".")
			if dotIdx != -1 && dotIdx+1 < len(funcName) {
				return funcName[dotIdx+1:]
			}
			return funcName
		}
	}
	return "unknown"
}

// 获取当前 goroutine ID
func getGoroutineID() string {
	// 通过 runtime.Stack hack 获取 goroutine id
	var buf [64]byte
	n := runtime.Stack(buf[:], false)
	stack := strings.TrimPrefix(string(buf[:n]), "goroutine ")
	idField := strings.Fields(stack)
	if len(idField) > 0 {
		return idField[0]
	}
	return "unknown"
}

func buildFields(entry *logrus.Entry) string {
	var fields []string
	for key, val := range entry.Data {
		stringVal, ok := val.(string)
		if !ok {
			stringVal = fmt.Sprint(val)
		}
		fields = append(fields, key+"="+stringVal)
	}
	return strings.Join(fields, ",")
}
