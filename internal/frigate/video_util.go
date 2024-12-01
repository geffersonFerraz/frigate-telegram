package frigate

import (
	"bytes"
	"errors"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

// VideoSplitOptions configura opções para divisão de vídeo
type VideoSplitOptions struct {
	MaxSizeBytes int64
	OutputFormat string
}

// SplitVideoWithFFmpeg divide um arquivo de vídeo usando FFmpeg
func SplitVideoWithFFmpeg(inputFile *os.File, options *VideoSplitOptions) ([]string, error) {
	// Configurações padrão se não forem fornecidas
	if options == nil {
		options = &VideoSplitOptions{
			MaxSizeBytes: 49 * 1024 * 1024, // 49 MB
			OutputFormat: "mp4",
		}
	}

	// Obter informações do arquivo de entrada
	inputFileInfo, err := inputFile.Stat()
	if err != nil {
		return nil, fmt.Errorf("erro ao obter informações do arquivo: %v", err)
	}

	// Obter nome base do arquivo
	baseName := inputFileInfo.Name()
	ext := filepath.Ext(baseName)
	baseNameWithoutExt := strings.TrimSuffix(baseName, ext)

	// Calcular o tempo total do vídeo
	durationCmd := exec.Command("ffprobe",
		"-v", "error",
		"-show_entries", "format=duration",
		"-of", "default=noprint_wrappers=1:nokey=1",
		inputFile.Name())

	var durationOutput bytes.Buffer
	durationCmd.Stdout = &durationOutput
	if err := durationCmd.Run(); err != nil {
		return nil, fmt.Errorf("erro ao obter duração do vídeo: %v", err)
	}

	// Converter duração para float
	durationStr := strings.TrimSpace(durationOutput.String())
	totalDuration, err := strconv.ParseFloat(durationStr, 64)
	if err != nil {
		return nil, fmt.Errorf("erro ao converter duração: %v", err)
	}

	// Calcular a duração máxima de cada parte, considerando o limite de 49MB por parte
	partDuration := totalDuration * float64(options.MaxSizeBytes) / float64(inputFileInfo.Size())

	segmentNumber := 1
	segmentStartTime := 0.0
	partsNames := []string{}
	// Para continuar dividindo até que o arquivo de entrada tenha sido completamente dividido
	for {
		outputFileName := fmt.Sprintf("/tmp/%s_part%d.%s", baseNameWithoutExt, segmentNumber, options.OutputFormat)

		// Comando FFmpeg para cortar vídeo com tamanho máximo de 49MB
		cmd := exec.Command("ffmpeg",
			"-i", inputFile.Name(),
			"-ss", fmt.Sprintf("%.2f", segmentStartTime), // Início do segmento
			"-t", fmt.Sprintf("%.2f", partDuration), // Duração do segmento
			"-c", "copy",
			"-fs", fmt.Sprintf("%d", options.MaxSizeBytes), // Limitar o tamanho do arquivo
			outputFileName)

		// Capturar possíveis erros
		var errOutput bytes.Buffer
		cmd.Stderr = &errOutput

		// Executar comando
		if err := cmd.Run(); err != nil {
			return nil, fmt.Errorf("erro ao dividir vídeo: %v - %s", err, errOutput.String())
		}

		partsNames = append(partsNames, outputFileName)

		// Atualizar o tempo de início do próximo segmento
		segmentStartTime += partDuration
		segmentNumber++

		if segmentStartTime >= totalDuration {
			break
		}
	}

	return partsNames, nil
}

// CleanupVideoFileParts fecha e remove todos os arquivos de partes
func CleanupVideoFileParts(files []*os.File) error {
	for _, file := range files {
		filename := file.Name()

		if err := file.Close(); err != nil {
			log.Printf("Erro ao fechar arquivo %s: %v", filename, err)
		}

		if err := os.Remove(filename); err != nil {
			log.Printf("Erro ao remover arquivo %s: %v", filename, err)
		}
	}
	return nil
}

// VerificarFFmpegInstalado verifica se o FFmpeg está instalado
func VerificarFFmpegInstalado() error {
	cmd := exec.Command("ffmpeg", "-version")
	if err := cmd.Run(); err != nil {
		return errors.New("FFmpeg não está instalado. Por favor, instale o FFmpeg primeiro")
	}
	return nil
}
