package task_01

import (
	"fmt"
	"strings"

	"devsec-contest01/internal/tasks/model"
)

const (
	id    = "noisy-ips"
	code  = "ЗАДАЧА 01"
	title = "Подозрительные IP"
)

const points = 125

const brief = `Наши аналитики заметили подозрительную активность с целого ряда IP адресов. Необходимо найти их в логах по заданным правилам, чтобы добавить эти адреса в черный список.`

var rules = []string{
	"Скачайте файл с логами",
	fmt.Sprintf(
		"Найдите IP адреса, которые имели >= %d событий в какую-либо секунду",
		threshold,
	),
	"Выведите найденные уникальные адреса построчно в отдельный .txt файл в любом порядке",
	"Прикрепите получившийся файл для проверки",
}

var sampleInput = strings.Repeat(
	`203.0.113.5 - - [10/May/2026:11:02:01 +0000] "GET /login HTTP/1.1" 401`+"\n",
	threshold,
) + `BROKEN RECORD
198.51.100.9 - - [10/May/2026:11:02:05 +0000] "GET /items HTTP/1.1" 200`

const sampleOutput = "203.0.113.5"

func description() model.Description {
	return model.Description{
		ID:           id,
		Code:         code,
		Title:        title,
		Points:       points,
		Brief:        brief,
		Rules:        rules,
		SampleInput:  sampleInput,
		SampleOutput: sampleOutput,
	}
}
