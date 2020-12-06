package main

import (
	"fmt"

	"strings"
	"time"

	"gopkg.in/guregu/null.v3"
)

func PrepareBatchValues(paramLength int, valueLength int) string {
	var valString string
	for i := 0; i < paramLength; i++ {
		valString = valString + "?,"
	}
	valString = fmt.Sprintf("(%s)", strings.TrimSuffix(valString, ","))

	var values string
	for i := 0; i < valueLength; i++ {
		values = fmt.Sprintf("%s, %s", values, valString)
	}
	return strings.TrimPrefix(values, ", ")
}

func PrepareBatchValuesPG(paramLength int, valueLength int) string {
	counter := 1
	var values string
	for i := 0; i < valueLength; i++ {
		values = fmt.Sprintf("%s, %s", values, genValString(paramLength, &counter))
	}
	return strings.TrimPrefix(values, ", ")
}

func genValString(paramLength int, counter *int) string {
	var valString string
	for i := 0; i < paramLength; i++ {
		valString = valString + fmt.Sprintf("$%d,", *counter)
		*counter++
	}
	valString = fmt.Sprintf("(%s)", strings.TrimSuffix(valString, ","))
	return valString
}

func GenTimeStamp(time time.Time) string {
	return time.Format("2006-01-02 15:04:05")
}

func GenNullTimeStamp() null.Time {
	time := time.Now()
	nullTime := null.NewTime(time, true)
	return nullTime
}
