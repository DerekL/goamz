package arn

import (
	//"fmt"
	"github.com/DerekL/goamz/aws"
	"strconv"
	"strings"
)

func BuildRDS(name string, accountno int) string {

	//arn:aws:rds:<region>:<account number>:<resourcetype>:<name>

	//http://docs.aws.amazon.com/AmazonRDS/latest/UserGuide/USER_Tagging.html#USER_Tagging.AR

	core := "arn:aws:rds"
	region := aws.USEast.Name
	//no dashes in account number!
	//account := "6610-9521-4357"
	//account := "661095214357"
	resourcetype := "db"
	s := []string{core, region, strconv.Itoa(accountno), resourcetype, name}
	arn := strings.Join(s, ":")

	return arn
}
