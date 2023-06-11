package main

import (
	"fmt"
	"github.com/gin-gonic/gin"
	"io"
	"net/http"
	"os/exec"
	"strings"
)

const defDockerCmd string  = `docker exec %s %s`


type GenericResponse struct {
	Status int            `json:"status"`
	Data []CronStatusResp `json:"data"`
}

type CronStatusResp struct {
	Container string `json:"container"`
	Count int `json:"count"`
	CronJobs []string `json:"cron_jobs"`
}

type CronSetReq struct {
	Containers []string `json:"containers"`
	CronJobs   []string `json:"cron_jobs"`
}


func MakeDockerCommand(container string, cmd string) string {
	return fmt.Sprintf(defDockerCmd, container, cmd)
}

func RemoveEmpty(items []string) []string {
	j := 0
	for i, v := range items {
		if v != "" {
			items[j] = items[i]
			j++
		}
	}
	items = items[:j]
	return items
}

func ExecCommand(cmd string, isNilOk bool, sep string) ([]string, bool) {
	var retVal string
	var flag bool

	if sep == "" {
		sep = "\n"
	}

	command := exec.Command("/bin/zsh", "-c", cmd)
	cmdOut, _ := command.StdoutPipe()
	cmdErr, _ := command.StderrPipe()

	command.Start()
	outBytes, _ := io.ReadAll(cmdOut)
	errBytes, _ := io.ReadAll(cmdErr)
	command.Wait()

	if len(errBytes) > 0 {
		err := string(errBytes)
		if !isNilOk {
			fmt.Println(err)
			return nil, true
		} else {
			flag = true
			retVal = err
		}
	} else {
		flag = false
		retVal = string(outBytes)
	}

	return strings.Split(retVal, sep), flag
}

func ActiveDockerContainers() ([]string, bool) {
	cmd := "docker ps --format {{.Names}}"
	output, _ := ExecCommand(cmd, false, "")
	if output == nil {
		return nil, true
	}

	output = RemoveEmpty(output)
	//n := len(output)

	//for n >= 0 && output[n-1] == "" {
	//	output = output[:n-1]
	//	n -= 1
	//}

	return output, false
}

func (resp *GenericResponse) ServerError() {
	resp.Status = http.StatusInternalServerError
	resp.Data = make([]CronStatusResp, 0)
	return
}

func ActiveCrons(container string) CronStatusResp {
	var output []string
	var isErr bool
	var count int
	var resp CronStatusResp

	cmd := "crontab -l"
	resp.Container = container
	resp.CronJobs = nil

	if container != "HOST" {
		output, isErr = ExecCommand(MakeDockerCommand(container, cmd), true, "")
	} else {
		output, isErr = ExecCommand(cmd, true, "")
	}
	if output == nil {
		return resp
	}

	output = RemoveEmpty(output)
	n := len(output)
	//for n >= 0 && output[n-1] == "" {
	//	output = output[:n-1]
	//	n -= 1
	//}

	if isErr {
		count = 0
	} else {
		count = n
	}

	resp.Count = count
	resp.CronJobs = output
	return resp
}

// FmtCronCmds TODO: add as closure
func FmtCronCmds(jobs []string) string {
	fmtJobs := ""

	for _, job := range jobs {
		fmtJobs += job
		fmtJobs += "\n"
	}

	return fmtJobs
}

func SetCronForContainer(containers []string, cronJobs []string) {
	for _, container := range containers {
		// TODO: check this
		if container != "HOST" {
			continue
		}

		cmd := "crontab -l | { cat; echo \"%s\"; } | crontab -"
		jobs := FmtCronCmds(cronJobs)

		ExecCommand(fmt.Sprintf(cmd, jobs), true, "")
	}
}

func DeleteCronForContainer(containers []string, cronJobs []string) {
	var fmtCmd = func(jobs []string) string {
		fmtJob := ""
		for _, job := range jobs {
			fmtJob += job
			fmtJob += "\\|"
		}

		return fmtJob[:len(fmtJob)-2]
	}

	for _, container := range containers {
		// TODO: check this
		if container != "HOST" {
			continue
		}

		cmd := "crontab -l | grep -v \"%s\" | crontab -"
		fmt.Println(fmt.Sprintf(cmd, fmtCmd(cronJobs)))
		ExecCommand(fmt.Sprintf(cmd, fmtCmd(cronJobs)), true, "")
	}
}

func SetCron(ctx *gin.Context) {
	var body CronSetReq
	if err := ctx.ShouldBindJSON(&body); err != nil {
		panic(err)
	}

	SetCronForContainer(body.Containers, body.CronJobs)

	ctx.IndentedJSON(http.StatusOK, gin.H{"message": "cron jobs set successfully"})
}

func DeleteCron(ctx *gin.Context) {
	var body CronSetReq
	if err := ctx.ShouldBindJSON(&body); err != nil {
		panic(err)
	}

	DeleteCronForContainer(body.Containers, body.CronJobs)

	ctx.IndentedJSON(http.StatusOK, gin.H{"message": "cron jobs deleted successfully"})
}

func AllActiveCrons(ctx *gin.Context) {
	var genResp GenericResponse
	var resp CronStatusResp

	activeContainers, _ := ActiveDockerContainers()
	if activeContainers == nil {
		genResp.ServerError()
		return
	}

	genResp.Status = http.StatusOK
	genResp.Data = make([]CronStatusResp, 0)

	n := len(activeContainers)
	for i := 0; i <= n; i++ {
		if i != n {
			resp = ActiveCrons(activeContainers[i])
		} else {
			resp = ActiveCrons("HOST")
		}

		if len(resp.CronJobs) < 1 {
			genResp.ServerError()
			break
		}
		genResp.Data = append(genResp.Data, resp)
	}

	ctx.IndentedJSON(http.StatusOK, genResp)
}

func ActiveContainerCrons(ctx *gin.Context) {
	var resp CronStatusResp
	var genResp GenericResponse

	resp = ActiveCrons(ctx.Param("name"))

	if len(resp.CronJobs) < 1 {
		genResp.ServerError()
	} else {
		genResp.Status = http.StatusOK
		genResp.Data = []CronStatusResp{resp}
	}

	ctx.IndentedJSON(http.StatusOK, genResp)
}


func main() {
	router := gin.Default()
	cronRouter := router.Group("/cron")
	{
		cronRouter.GET("/", AllActiveCrons)
		cronRouter.GET("/container/:name", ActiveContainerCrons)
		cronRouter.POST("/set-cron", SetCron)
		cronRouter.DELETE("/delete-cron", DeleteCron)
	}

	router.Run(":9000")
}
