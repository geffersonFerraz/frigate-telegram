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
func SplitVideoWithFFmpeg(inputFile *os.File, options *VideoSplitOptions) (map[string]*os.File, error) {
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

	// Obter duração total do vídeo
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

	// Calcular número de partes
	var outputFiles = make(map[string]*os.File)
	segmentDuration := float64(0)
	segmentNumber := 1

	// Executar divisão com FFmpeg
	for segmentDuration < totalDuration {
		outputFileName := fmt.Sprintf("/tmp/%s_part%d.%s", baseNameWithoutExt, segmentNumber, options.OutputFormat)

		// Comando FFmpeg para cortar vídeo
		cmd := exec.Command("ffmpeg",
			"-i", inputFile.Name(),
			"-t", fmt.Sprintf("%.2f", totalDuration-segmentDuration),
			"-c", "copy",
			outputFileName)

		// Capturar possíveis erros
		var errOutput bytes.Buffer
		cmd.Stderr = &errOutput

		// Executar comando
		if err := cmd.Run(); err != nil {
			return nil, fmt.Errorf("erro ao dividir vídeo: %v - %s", err, errOutput.String())
		}

		// Abrir arquivo gerado
		outputFile, err := os.Open(outputFileName)
		if err != nil {
			return nil, fmt.Errorf("erro ao abrir arquivo de saída: %v", err)
		}
		outputFiles[outputFileName] = outputFile

		// Verificar tamanho do arquivo
		outputFileInfo, err := outputFile.Stat()
		if err != nil {
			return nil, fmt.Errorf("erro ao obter informações do arquivo de saída: %v", err)
		}

		// Atualizar duração e número do segmento
		segmentDuration += float64(outputFileInfo.Size()) / float64(inputFileInfo.Size()) * totalDuration
		segmentNumber++

		// Parar se o próximo segmento ultrapassar o tamanho máximo
		if outputFileInfo.Size() > options.MaxSizeBytes {
			break
		}
	}

	return outputFiles, nil
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
