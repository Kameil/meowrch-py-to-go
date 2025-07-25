package main

// original code was created by DIMFLIX (Modified by K1rsN7) (rewritten in go by Kameil)
// Github: https://github.com/DIMFLIX

import (
	"bufio"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"gopkg.in/ini.v1"

	"github.com/lichunqiang/gputil"
	"github.com/shirou/gopsutil/v4/cpu"
	"github.com/shirou/gopsutil/v4/mem"
)

type CpuInfo struct {
	Percent  float64
	Temp     float64
	Name     string
	Critical bool
}

type RamInfo struct {
	Total    string
	Icon     string
	Used     float64
	Percent  float64
	Critical bool
}

type GpuInfo struct {
	Name    string
	GpuLoad int
	GpuTemp int
}

type Valicons struct {
	percentIcon     string
	percentCritical bool
	tempIcon        string
	tempCritical    bool
}

func getIcons(percentVal float64, tempVal float64) Valicons {
	percentCritical := false
	tempCritical := false
	var percentIcon, tempIcon string

	if percentVal < 40 {
		percentIcon = "󰾆 "
	} else if percentVal < 70 {
		percentIcon = "󰾅 "
	} else if percentVal < 90 {
		percentIcon = "󰓅 "
	} else {
		percentIcon = " "
		percentCritical = true
	}
	if tempVal < 40 {
		tempIcon = " "
	} else if tempVal < 70 {
		tempIcon = " "
	} else if tempVal < 90 {
		tempIcon = " "
	} else {
		tempIcon = " "
		tempCritical = true
	}

	return Valicons{
		percentIcon:     percentIcon,
		percentCritical: percentCritical,
		tempIcon:        tempIcon,
		tempCritical:    tempCritical,
	}
}

func cpuSearchThermalPathIntel() (string, error) {
	thermalPath := "/sys/class/thermal"

	dirs, err := os.ReadDir(thermalPath)
	if err != nil {
		return "", fmt.Errorf(`Error reading "/sys/class/thermal"thermal directory: %w`, err)
	}

	re := regexp.MustCompile(`^thermal_zone\d+$`)
	for _, dir := range dirs {
		if re.MatchString(dir.Name()) {
			file, err := os.ReadFile(filepath.Join(thermalPath, dir.Name(), "type"))
			if err != nil {
				return "", fmt.Errorf("error reading type of %s: %w", dir.Name(), err)
			}
			thermalType := strings.TrimSpace(string(file))
			if thermalType == "x86_pkg_temp" {
				return filepath.Join(thermalPath, dir.Name()), nil
			}
		}
	}
	return "", fmt.Errorf("Thermal_zone not found. thermal_path=\"%s\"", thermalPath)

}

func getCPUTempDirect() (float64, error) {
	thermalPath, err := cpuSearchThermalPathIntel()
	if err != nil {
		return 0, err
	}
	data, err := os.ReadFile(filepath.Join(thermalPath, "temp"))
	if err != nil {
		return 0, err
	}
	tempStr := strings.TrimSpace(string(data))
	tempMilli, err := strconv.ParseFloat(tempStr, 64)
	if err != nil {
		return 0, err
	}
	return tempMilli / 1000.0, nil
}

func CpuGetInfo() CpuInfo {
	Percent, err := cpu.Percent(time.Second, false)
	if err != nil {
		log.Println("CPU: error getting cpu usage percent: ", err)
		panic(err)
	}
	file, err := os.Open("/proc/cpuinfo")
	if err != nil {
		log.Println("CPU: error opening file /proc/cpuinfo: ", err)
		panic(err)
	}
	defer file.Close()

	reader := bufio.NewReader(file)
	var cpuName string
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			if err.Error() == "EOF" {
				break
			}
			log.Println("CPU: error reading line in file /proc/cpuinfo: ", err)
			panic(fmt.Errorf("CPU: error reading line in file /proc/cpuinfo: %w", err))
		}
		if strings.Contains(line, "name") {
			line = strings.Split(line, ":")[1]
			line = strings.TrimSpace(line)
			cpuName = line
			break
		}
	}

	temp, err := getCPUTempDirect()
	if err != nil {
		log.Println("CPU: erro getting temperature: ", err)
		temp = 0.0
	}
	return CpuInfo{
		Percent:  Percent[0],
		Temp:     temp,
		Name:     cpuName,
		Critical: false,
	}

}
func RamGetInfo() RamInfo {
	svmem, err := mem.VirtualMemory()
	if err != nil {
		log.Println(`RAM: error in "svmem, err := mem.VirtualMemory()"\nerror getting RAM information: `, err)
		panic(fmt.Errorf("error getting RAM information: %w", err))
	}

	total := fmt.Sprintf("%.2f", float64(svmem.Total)/(1024.0*1024.0*1024.0))
	used := round(float64(svmem.Used)/(1024.0*1024.0*1024.0), 2)
	percent := svmem.UsedPercent
	critical := false
	icon := ""

	switch {
	case percent < 40:
		icon = "󰾆 "
	case percent < 70:
		icon = "󰾅 "
	case percent < 90:
		icon = "󰓅 "
	default:
		icon = " "
		critical = true
	}

	return RamInfo{
		Total:    total,
		Icon:     icon,
		Used:     used,
		Percent:  percent,
		Critical: critical,
	}
}

func round(val float64, precision int) float64 {
	factor := math.Pow(10, float64(precision))
	return math.Round(val*factor) / factor
}

func searchGpuPath() (string, error) {
	drmPath := "/sys/class/drm"
	dirs, err := os.ReadDir(drmPath)
	if err != nil {
		return "", fmt.Errorf("Error reading /sys/class/drm directory: %w", err)
	}
	re := regexp.MustCompile(`^card\d+$`)
	for _, dir := range dirs {

		fullPath := filepath.Join(drmPath, dir.Name())

		info, err := os.Stat(fullPath)
		if err != nil {
			continue
		}

		if info.IsDir() {
			if re.MatchString(dir.Name()) {
				gpuPath := filepath.Join(drmPath, dir.Name())
				return gpuPath, nil
			}
		}
	}
	return "", fmt.Errorf("Path to amdgpu not found. drm_path=\"%s\"", drmPath)

}

func gpuGetHwmon(gpuPath string) (string, error) {
	dirs, err := os.ReadDir(filepath.Join(gpuPath, "device", "hwmon"))
	if err != nil {
		return "", fmt.Errorf("error getting gpu hwmon directory from gpu path: %w", err)
	}
	var hwmonName string = "hwmon0"
	for _, dir := range dirs {
		if strings.Contains(dir.Name(), "hwmon") {
			hwmonName = dir.Name()
			return hwmonName, nil
		}
	}
	return "", fmt.Errorf("unable to found gpu hwmon directory from gpu path")

}

func hwmonGetFirstTempInput(hwmonName string) (string, error) {
	hwmonDirectory := filepath.Join("/sys/class", "hwmon", hwmonName)
	files, err := os.ReadDir(hwmonDirectory)
	if err != nil {
		return "", fmt.Errorf("error reading hwmon directory. hwmonDirectory = \"%s\": %w", hwmonDirectory, err)
	}
	re := regexp.MustCompile(`^temp\d+_input`)
	for _, file := range files {
		if re.MatchString(file.Name()) {
			return file.Name(), nil
		}
	}
	return "", fmt.Errorf(`temp\d+_input not found in hwmon directory %s`, hwmonDirectory)
}

func GpuGetInfo() GpuInfo {

	gpuPath, err := searchGpuPath()
	if err != nil {
		log.Println("GPU - AMD: ", err)
		panic(err)
	}
	gpuHwmonName, err := gpuGetHwmon(gpuPath)
	if err != nil {
		log.Println("GPU - AMD: ", err)
		panic(err)
	}
	gpuTempInput, err := hwmonGetFirstTempInput(gpuHwmonName)
	if err != nil {
		log.Println("GPU - AMD: ", err)
		panic(err)
	}
	ctx := context.Background()
	gpus, err := gputil.GetGPUs(ctx)
	if err != nil {
		gpuName := "N/A"
		file, err := os.ReadFile(filepath.Join(gpuPath, "device", "hwmon", gpuHwmonName, gpuTempInput))
		if err != nil {
			log.Printf("GPU - AMD: error reading gpu temp input %s: %v", filepath.Join(gpuPath, "device", "hwmon", gpuHwmonName, "temp1_input"), err)
			panic(err)
		}
		gpuTempValStr := strings.TrimSpace(string(file))
		gpuTempValInt, err := strconv.Atoi(gpuTempValStr)
		if err != nil {
			log.Println("GPU - AMD: error converting gpu temp_input value to int: ", err)
			panic(err)
		}
		gpuTemp := gpuTempValInt / 1000

		percentFile, err := os.ReadFile(filepath.Join(gpuPath, "device", "gpu_busy_percent"))
		if err != nil {
			log.Printf("GPU - AMD: error reading gpu busy percent file %s: %v", filepath.Join(gpuPath, "device", "gpu_busy_percent"), err)
			panic(err)
		}
		percentValStr := strings.TrimSpace(string(percentFile))
		percentValInt, err := strconv.Atoi(percentValStr)

		gpuUsage := percentValInt

		return GpuInfo{
			Name:    gpuName,
			GpuLoad: gpuUsage,
			GpuTemp: gpuTemp,
		}

	} else {
		gpu := gpus[0]
		gpuName := gpu.Name
		percentStr, err := strconv.Atoi(gpu.UtilizationGPU)
		if err != nil {
			log.Println("GPU - NVIDIA: error converting gpu utilization to int: ", err)
			panic(fmt.Errorf("error converting gpu utilization to int: %w", err))
		}
		gpuTemp, err := strconv.Atoi(gpu.Temperature)
		if err != nil {
			log.Println("GPU - NVIDIA: error converting gpu temperature to int: ", err)
			panic(fmt.Errorf("error converting gpu temperature to int: %w", err))
		}
		return GpuInfo{
			Name:    gpuName,
			GpuLoad: percentStr,
			GpuTemp: gpuTemp,
		}
	}

}

func GetSystemInfoConfig(configPath string) (string, string) {
	dir := filepath.Dir(configPath)
	_ = os.MkdirAll(dir, os.ModePerm)

	var cfg *ini.File
	var err error

	if _, err = os.Stat(configPath); os.IsNotExist(err) {
		cfg = ini.Empty()
		cfg.Section("DEFAULT").Key("cpu-label-mode").SetValue("utilization")
		cfg.Section("DEFAULT").Key("gpu-label-mode").SetValue("utilization")
		err = cfg.SaveTo(configPath)
		if err != nil {
			panic(err)
		}
		return "utilization", "utilization"
	}

	cfg, err = ini.Load(configPath)
	if err != nil {
		panic(err)
	}

	cpuMode := cfg.Section("DEFAULT").Key("cpu-label-mode").MustString("utilization")
	gpuMode := cfg.Section("DEFAULT").Key("gpu-label-mode").MustString("utilization")

	if cpuMode != "utilization" && cpuMode != "temp" {
		cpuMode = "utilization"
	}
	if gpuMode != "utilization" && gpuMode != "temp" {
		gpuMode = "utilization"
	}

	return cpuMode, gpuMode
}

func SetSystemInfoConfig(configPath, cpuMode, gpuMode string) {
	dir := filepath.Dir(configPath)
	_ = os.MkdirAll(dir, os.ModePerm)

	cfg := ini.Empty()
	if _, err := os.Stat(configPath); err == nil {
		cfg, _ = ini.Load(configPath)
	}

	if cpuMode != "utilization" && cpuMode != "temp" {
		cpuMode = "utilization"
	}
	if gpuMode != "utilization" && gpuMode != "temp" {
		gpuMode = "utilization"
	}

	cfg.Section("DEFAULT").Key("cpu-label-mode").SetValue(cpuMode)
	cfg.Section("DEFAULT").Key("gpu-label-mode").SetValue(gpuMode)

	err := cfg.SaveTo(configPath)
	if err != nil {
		panic(err)
	}
}

func main() {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		panic(fmt.Errorf("Main: Error getting User home directory: %w", err))
	}

	systemConfigPath := filepath.Join(homeDir, ".cache/meowrch/system-info.ini")
	cpuMode, gpuMode := GetSystemInfoConfig(systemConfigPath)
	logFile, err := os.OpenFile(filepath.Join(homeDir, ".cache", "meowrch", "system-info.log"), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0666)
	if err != nil {
		log.Fatal("error opening log file:", err)
	}
	log.SetOutput(logFile)

	val := os.ExpandEnv("$XDG_SESSION_TYPE")
	var sessionType *string

	if val != "$XDG_SESSION_TYPE" && val != "" {
		sessionType = &val
	}

	cpu := flag.Bool("cpu", false, "store_true")
	ram := flag.Bool("ram", false, "store_true")
	gpu := flag.Bool("gpu", false, "store_true")
	click := flag.Bool("click", false, "store_true")
	normalColor := flag.String("normal-color", "#a6e3a1", "string")
	criticalColor := flag.String("critical-color", "#f38ba8", "string")

	flag.Parse()
	if *cpu {
		if *click {
			if cpuMode == "temp" {
				SetSystemInfoConfig(systemConfigPath, "utilization", gpuMode)
			} else {
				SetSystemInfoConfig(systemConfigPath, "temp", gpuMode)
			}
		} else {
			cpuInfo := CpuGetInfo()
			icons := getIcons(cpuInfo.Percent, cpuInfo.Temp)
			var color string
			if icons.percentCritical || icons.tempCritical {
				color = *criticalColor
			} else {
				color = *normalColor
			}
			var cpuText string
			if cpuMode == "temp" {
				cpuText = fmt.Sprintf("󰍛 %.0f°C", cpuInfo.Temp)
			} else {
				cpuText = fmt.Sprintf("󰍛 %.0f%%", cpuInfo.Percent)
			}

			if sessionType != nil {
				switch *sessionType {
				case "x11":
					text := fmt.Sprintf("%%{F%s}%s%%{F-}", color, cpuText)
					fmt.Println(text)
				case "wayland":
					data := map[string]string{
						"text":    fmt.Sprintf("<span color=\"%s\">%s</span>", color, cpuText),
						"tooltip": fmt.Sprintf("󰍛 Name: %s\n%sUtilization: %.0f%%\n%sTemp: %.0f°C", cpuInfo.Name, icons.percentIcon, cpuInfo.Percent, icons.tempIcon, cpuInfo.Temp),
					}
					jsonBytes, err := json.Marshal(data)
					if err != nil {
						panic(err)
					}

					fmt.Println(string(jsonBytes))
				}
			}

		}
	} else if *ram {
		ramInfo := RamGetInfo()
		var color string
		if ramInfo.Critical {
			color = *criticalColor
		} else {
			color = *normalColor
		}

		if sessionType != nil {
			switch *sessionType {
			case "x11":
				text := fmt.Sprintf("%s %.2f GB", ramInfo.Icon, ramInfo.Used)
				result := fmt.Sprintf(`%%{F%s}%s%%{F-}`, color, text)
				fmt.Print(result)
			case "wayland":
				data := map[string]string{
					"text": fmt.Sprintf("<span color=\"%s\">%s %.2f GB</span>", color, ramInfo.Icon, ramInfo.Used),
					"tooltip": fmt.Sprintf("%sPercent Utilization: %.2f%%\n  Utilization: %.2f/%s GB",
						ramInfo.Icon, ramInfo.Percent, ramInfo.Used, ramInfo.Total),
				}

				jsonBytes, err := json.Marshal(data)
				if err != nil {
					panic(err)
				}

				fmt.Println(string(jsonBytes))
			}
		}
	} else if *gpu {
		gpuInfo := GpuGetInfo()
		if *click {
			if gpuMode == "temp" {
				SetSystemInfoConfig(systemConfigPath, cpuMode, "utilization")
			} else {
				SetSystemInfoConfig(systemConfigPath, cpuMode, "temp")
			}
		} else {
			icons := getIcons(float64(gpuInfo.GpuLoad), float64(gpuInfo.GpuTemp))
			var color string
			if icons.percentCritical || icons.tempCritical {
				color = *criticalColor
			} else {
				color = *normalColor
			}
			var gpuText string
			if gpuMode == "temp" {
				gpuText = fmt.Sprintf("󰢮 %d°C", gpuInfo.GpuTemp)
			} else {
				gpuText = fmt.Sprintf("󰢮 %d%%", gpuInfo.GpuLoad)
			}
			if sessionType != nil {
				switch *sessionType {
				case "x11":
					result := fmt.Sprintf("%%{F%s}%s%%{F-}", color, gpuText)
					fmt.Println(result)
				case "wayland":
					data := map[string]string{
						"text": fmt.Sprintf("<span color=\"%s\">%s</span>", color, gpuText),
						"tooltip": fmt.Sprintf("󰢮 Name: %s\n%sUtilization: %d%%\n%sTemp: %d°C",
							gpuInfo.Name, icons.percentIcon, gpuInfo.GpuLoad, icons.tempIcon, gpuInfo.GpuTemp),
					}
					jsonBytes, err := json.Marshal(data)
					if err != nil {
						panic(err)
					}
					fmt.Println(string(jsonBytes))
				}
			}

		}

	} else {
		fmt.Println("Enter one of the arguments:\n--cpu to get information about the processor\n--ram to get information about RAM\n--gpu to get information about the graphics card")
	}

}
