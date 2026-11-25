package task_02

import "devsec-contest01/internal/tasks/model"

const (
	id    = "password-validator"
	code  = "ЗАДАЧА 02"
	title = "Проверка паролей"
)

const points = 100

const brief = `В ходе аудита выяснилось, что пароли некоторых наших пользователей не соответствуют требованиям ИБ. Необходимо выделить такие пароли, чтобы провести их ротацию.`

var rules = []string{
	"Скачайте файл с паролями, найдите те, которые не соответствуют требованиям ниже",
	"Длина от 8 до 64 символов включительно",
	"Хотя бы одна заглавная буква A–Z",
	"Хотя бы одна строчная буква a–z",
	"Хотя бы одна цифра 0–9",
	"Хотя бы один символ из !@#$%^&*()-_=+",
	"Без пробельных символов",
	"Список невалидных паролей выведите построчно в отдельный .txt файл в любом порядке",
	"Прикрепите получившийся файл для проверки",
}

const sampleInput = `Spender2026!
Ab1!
NoDigitsHere!
HasSpace 123A!`

const sampleOutput = `Ab1!
NoDigitsHere!
HasSpace 123A!`

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
