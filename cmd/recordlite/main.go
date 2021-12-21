package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"os"

	"github.com/fsaintjacques/recordlite"
)

func main() {
	var err error
	file := os.Stdin

	if len(os.Args) > 0 {
		path := os.Args[1]
		file, err = os.Open(path)
		if err != nil {
			log.Fatalf("Failed opening path '%s': %s", path, err.Error())
		}
	}
	defer file.Close()

	payload, err := ioutil.ReadAll(file)
	if err != nil {
		log.Fatalf("Failed reading file: %s", err.Error())
	}

	var view recordlite.ViewDef
	json.Unmarshal(payload, &view)

	statement, err := recordlite.CompileViewDef(&view)
	if err != nil {
		log.Fatalf("Failed compiling table: %s", err.Error())
	}

	fmt.Println(statement)
}
