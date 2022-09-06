package utils

import (
	"bytes"
	"fmt"
	"log"
	"os/exec"
)

//ExecBashCmd 执行具体命令的封装
func ExecBashCmd(command string) (string, error) {
	log.Println(fmt.Sprintf("cli:bash -c %s", command))
	var out, stdrrr bytes.Buffer
	cmd := exec.Command("bash", "-c", command)
	cmd.Stdout = &out
	cmd.Stderr = &stdrrr
	err := cmd.Run()
	if err != nil {
		log.Println(fmt.Sprintf("执行命令[bash -c '%s']发生错误:%v,output:%v", command, stdrrr.String(), out.String()))
		return out.String(), err
	}
	return out.String(), nil
}
