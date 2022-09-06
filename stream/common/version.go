package common

import "fmt"

var (
	AgentBuild     = "2022.0820.000"
	AgentVersion   = "v2.1"
	AgentCopyright = "tdhxkj.com"
)

func GetAgent() string {
	return fmt.Sprintf("%s/%s(%s)", AgentCopyright, AgentVersion, AgentBuild)
}
