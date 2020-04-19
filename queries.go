package main

import (
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"
	"sync"
)

type Queries struct {
	mx  sync.RWMutex
	set []*Search
}

func (q *Queries) Add(s *Search) {
	q.mx.Lock()
	defer q.mx.Unlock()

	q.set = append((*q).set, s)
}

func (q *Queries) List(w http.ResponseWriter) {
	q.mx.RLock()
	defer q.mx.RUnlock()

	// Чертим список запросов
	if q.set == nil {
		fmt.Println("Запросов пока небыло")
		return
	}

	var err error

	fmt.Println("Найдено:", len((*q).set), "URL")
	for key, value := range (*q).set {

		tmpString := strings.Join([]string{"Запрос строки ", (*value).Search, ". Кол-во URL:", strconv.Itoa(len((*value).URLs)), ". <a href=/url/", strconv.Itoa(key), ">Смотреть результаты</a><br>"},"")
		_, err = fmt.Fprintln(w, tmpString)
		if err != nil {
			log.Println(err)
			return
		}
	}
}

func (q *Queries) GetOne(w http.ResponseWriter, id byte) {
	q.mx.RLock()
	defer q.mx.RUnlock()

	if !(*q).set[id].Finished {
		_, err := fmt.Fprintln(w, "Обработка ещё не завершена")
		if err != nil {
			log.Println(err)
		}
		return
	}

	_, err := fmt.Fprintln(w, "Cтрока поиска: ", (*q).set[id].Search, ".<br><br><b>Список URL в которых URL встречается:</b><br>")
	if err != nil {
		log.Println(err)
	}

	// обходим URL в наборе
	for _, value := range (*q).set[id].URLs {
		_, err := fmt.Fprintln(w, "<a href=", value, ">", value, "</a><br>")
		if err != nil {
			log.Println(err)
			return
		}
	}

	_, err = fmt.Fprintln(w, "<br><a href=/result/check>Список результатов</a>")
	if err != nil {
		log.Println(err)
	}

	_, err = fmt.Fprintln(w, "<br><br><a href=/>Новый запрос</a>")
	if err != nil {
		log.Println(err)
	}
}
