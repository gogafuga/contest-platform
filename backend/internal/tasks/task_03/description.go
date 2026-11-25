package task_03

import (
	"devsec-contest01/internal/tasks/model"
	"fmt"
)

const (
	id    = "cidr-check"
	code  = "ЗАДАЧА 03"
	title = "Запрещённые IP"
)

const points = 150

const allowedCIDR = "172.16.32.0/20"

const brief = `Кажется, наш файервол неправильно настроен: к нам приходят запросы с запрещённых IP. Чтобы проверить это, найдите адреса, доступ которым должен быть запрещён.`

var rules = []string{
	fmt.Sprintf("Доступ разрешён только IP-адресам из подсети %s", allowedCIDR),
	"Скачайте файл с IP-адресами: по одному адресу в строке. Все адреса — корректные IPv4",
	"Адрес сети и широковещательный адрес тоже входят в разрешённую подсеть",
	"Найдите IP-адреса, которые не входят в разрешённую подсеть",
	"Выведите найденные адреса без повторений построчно в отдельный .txt файл в любом порядке",
	"Прикрепите получившийся файл для проверки",
}

const sampleInput = `172.16.32.10
172.16.47.255
172.16.48.1
192.0.2.1`

const sampleOutput = `172.16.48.1
192.0.2.1`

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
