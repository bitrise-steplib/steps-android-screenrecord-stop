package main

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"

	"github.com/bitrise-io/go-utils/command"
	"github.com/bitrise-io/go-utils/log"
	"github.com/bitrise-io/go-utils/pathutil"
	"github.com/bitrise-tools/go-steputils/input"
)

// ConfigsModel ...
type ConfigsModel struct {
	RecordID       string
	RemoveFrames   string
	EmulatorSerial string
}

type adbModel struct {
	adbBinPath string
	serial     string
}

func createConfigsModelFromEnvs() ConfigsModel {
	return ConfigsModel{
		RecordID:       os.Getenv("record_id"),
		EmulatorSerial: os.Getenv("emulator_serial"),
		RemoveFrames:   os.Getenv("remove_frames"),
	}
}

func (configs ConfigsModel) print() {
	log.Infof("Configs:")
	log.Printf("- RecordID: %s", configs.RecordID)
	log.Printf("- RemoveFrames: %s", configs.RemoveFrames)
	log.Printf("- EmulatorSerial: %s", configs.EmulatorSerial)
}

func (configs ConfigsModel) validate() error {
	if err := input.ValidateIfNotEmpty(configs.RecordID); err != nil {
		return fmt.Errorf("RecordID, error: %s", err)
	}
	if err := input.ValidateIfNotEmpty(configs.RemoveFrames); err != nil {
		return fmt.Errorf("RemoveFrames, error: %s", err)
	}
	if err := input.ValidateIfNotEmpty(configs.EmulatorSerial); err != nil {
		return fmt.Errorf("EmulatorSerial, error: %s", err)
	}

	return nil
}

func (model adbModel) pull(commands ...string) (string, error) {
	cmd := command.New(model.adbBinPath, append([]string{"-s", model.serial, "pull"}, commands...)...)
	return cmd.RunAndReturnTrimmedCombinedOutput()
}

func (model adbModel) shell(commands ...string) (string, error) {
	cmd := command.New(model.adbBinPath, append([]string{"-s", model.serial, "shell"}, commands...)...)
	return cmd.RunAndReturnTrimmedCombinedOutput()
}

func (model adbModel) shellDetached(commands ...string) (string, error) {
	cmd := command.New(model.adbBinPath, append([]string{"-s", model.serial, "shell"}, commands...)...)
	rCmd := cmd.GetCmd()
	var b bytes.Buffer
	rCmd.Stdout = &b
	rCmd.Stderr = &b
	err := rCmd.Start()
	return b.String(), err
}

func mainE() error {
	// Input validation
	configs := createConfigsModelFromEnvs()

	fmt.Println()
	configs.print()

	if err := configs.validate(); err != nil {
		log.Errorf("Issue with input: %s", err)
		os.Exit(1)
	}

	fmt.Println()

	//
	// Main
	log.Infof("Checking compability")
	androidHome := os.Getenv("ANDROID_HOME")
	if androidHome == "" {
		return fmt.Errorf("no ANDROID_HOME set")
	}
	adbBinPath := filepath.Join(androidHome, "platform-tools/adb")
	exists, err := pathutil.IsPathExists(adbBinPath)
	if err != nil {
		return fmt.Errorf("failed to check if path exists: %s, error: %s", adbBinPath, err)
	}
	if !exists {
		return fmt.Errorf("adb binary doesn't exist at: %s", adbBinPath)
	}

	adb := adbModel{adbBinPath: adbBinPath, serial: configs.EmulatorSerial}

	out, err := adb.shell("which screenrecord")
	if err != nil {
		return fmt.Errorf("failed to run adb command, error: %s, output: %s", err, out)
	}
	if out == "" {
		return fmt.Errorf("screenrecord binary is not available on the device")
	}
	out, err = adb.shell("ps | grep screenrecord")
	if err != nil {
		return fmt.Errorf("failed to run adb command or screenrecord is not running on the device, error: %s, output: %s", err, out)
	}

	log.Donef("- Done")
	fmt.Println()

	log.Infof("Stop recording")
	_, err = adb.shell("killall -INT screenrecord && while [ \"$(pgrep screenrecord)\" != \"\" ]; do sleep 1; done")
	if err != nil {
		return fmt.Errorf("failed to run adb command, error: %s, output: %s", err, out)
	}

	log.Printf("- Check if screen recording stopped")
	out, err = adb.shell("ps | grep screenrecord | cat")
	if err != nil {
		return fmt.Errorf("failed to run adb command, error: %s, output: %s", err, out)
	}

	if out != "" {
		return fmt.Errorf("screenrecord still running, out: %s", out)
	}

	log.Donef("- Stopped")

	fmt.Println()

	log.Infof("Pulling video")

	deployDir := os.Getenv("BITRISE_DEPLOY_DIR")
	exportedPath := filepath.Join(deployDir, fmt.Sprintf("%s.mp4", configs.RecordID))

	_, err = adb.pull(fmt.Sprintf("/data/local/tmp/%s.mp4", configs.RecordID), exportedPath)
	if err != nil {
		return fmt.Errorf("failed to run adb command, error: %s, output: %s", err, out)
	}

	log.Donef("- Done")

	if configs.RemoveFrames == "true" {
		fmt.Println()

		log.Infof("Remove duplicated frames")

		trimmedExportedPath := filepath.Join(deployDir, fmt.Sprintf("%s_trimmed.mp4", configs.RecordID))

		trimCommand := command.New("ffmpeg", "-i", exportedPath, "-vf", "mpdecimate,setpts=N/FRAME_RATE/TB", trimmedExportedPath)

		trimCommand.SetStdout(os.Stdout)
		trimCommand.SetStderr(os.Stderr)

		err = trimCommand.Run()
		if err != nil {
			return fmt.Errorf("failed to run ffmpeg command, error: %s", err)
		}

		err = os.RemoveAll(exportedPath)
		if err != nil {
			return fmt.Errorf("failed to remove file(%s), error: %s", exportedPath, err)
		}

		log.Donef("- Done")
	}

	return nil
}

func main() {
	err := mainE()
	if err != nil {
		log.Errorf("Error: %v", err)
		os.Exit(1)
	}
}
