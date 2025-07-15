package common

import (
	"encoding/json"
	"fmt"
)

func PrintRespJSON(resp interface{}) {
	jsonBytes, err := json.MarshalIndent(resp, "", "    ")
	if err != nil {
		fmt.Println("unable to decode response: ", err)

		return
	}

	fmt.Printf("%s\n", jsonBytes)
}
