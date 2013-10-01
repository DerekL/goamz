//
// goamz - Go packages to interact with the Amazon Web Services.
//
//   https://wiki.ubuntu.com/goamz
//
// Copyright (c) 2011 Memeo Inc.
//
// Written by Prudhvi Krishna Surapaneni <me@prudhvi.net>
//Hacked by Derek Leuridan (derek@leuridanlabs.com)
// This package is in an experimental state, and does not currently
// follow conventions and style of the rest of goamz or common
// Go conventions. It must be polished before it's considered a
// first-class package in goamz.

package rds

import (
	//"crypto/rand"
	//"encoding/hex"
	"encoding/xml"
	"fmt"
	"github.com/DerekL/goamz/aws"
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"
	"sort"
	"strconv"
	"time"
)

const debug = false

// The RDS type encapsulates operations with a specific RDS region.
type RDS struct {
	aws.Auth
	aws.Region
	private byte // Reserve the right of using private data.
}

// New creates a new RDS.
func New(auth aws.Auth, region aws.Region) *RDS {
	return &RDS{auth, region, 0}
}

type RDSDescribeParamGroupsResp struct {
	ParameterGroups []ParameterGroup `xml:"DescribeDBParameterGroupsResult>DBParameterGroups>DBParameterGroup"`
}

//  Struct allowing for capture of parameter group info
type ParameterGroup struct {
	Family      string `xml:"DBParameterGroupFamily"`
	Description string `xml:"Description"`
	Name        string `xml:"DBParameterGroupName"`
}
type RDSDescribeParamsResp struct {
	Parameters []Parameter `xml:"DescribeDBParametersResult>Parameters>Parameter"`
}

type Parameter struct {
	ParameterValue string `xml:"ParameterValue"`
	DataType       string `xml:"DataType"`
	Sourcesystem   string `xml:"Sourcesystem"`
	IsModifiable   string `xml:"IsModifiable"`
	Description    string `xml:"Description "`
	ApplyType      string `xml:"ApplyType "`
	ParameterName  string `xml:"ParameterName"`
}

type RDSResp struct {
	Instances []DBInstance `xml:"DescribeDBInstancesResult>DBInstances>DBInstance"`
}

type DBInstance struct {
	LatestRestorableTime       string `xml:"LatestRestorableTime"`
	Engine                     string `xml:"Engine"`
	PendingModifiedValues      string `xml:"PendingModifiedValues"`
	BackupRetentionPeriod      string `xml:"BackupRetentionPeriod"`
	MultiAZ                    string `xml:"MultiAZ"`
	LicenseModel               string `xml:"LicenseModel"`
	DBInstanceStatus           string `xml:"DBInstanceStatus"`
	EngineVersion              string `xml:"EngineVersion"`
	EndpointPort               string `xml:"Endpoint>Port"`
	EndpointAddress            string `xml:"Endpoint>Address"`
	InstanceIdentifier         string `xml:"DBInstanceIdentifier"`
	SecurityGroupStatus        string `xml:"DBSecurityGroups>DBSecurityGroup>Status"`
	SecurityGroupName          string `xml:"DBSecurityGroups>DBSecurityGroup>DBSecurityGroupName"`
	PreferredBackupWindow      string `xml:"PreferredBackupWindow"`
	AutoMinorVersionUpgrade    string `xml:"AutoMinorVersionUpgrade"`
	PreferredMaintenanceWindow string `xml:"PreferredMaintenanceWindow"`
	AvailabilityZone           string `xml:"AvailabilityZone"`
	InstanceCreateTime         string `xml:"InstanceCreateTime"`
	AllocatedStorage           string `xml:"AllocatedStorage"`
	DBInstanceClass            string `xml:"DBInstanceClass"`
	MasterUsername             string `xml:"MasterUsername"`
}

func (RDS *RDS) DescribeInstances(instIds []string, filter *Filter) (resp *RDSResp, err error) {
	params := makeParams("DescribeDBInstances")
	addParamsList(params, "InstanceId", instIds)
	filter.addParams(params)
	resp = &RDSResp{}
	err = RDS.query(params, resp)
	if err != nil {
		return nil, err
	}
	return
}

func (RDS *RDS) DescribeDBParameterGroups(groupNames []string, filter *Filter) (resp *RDSDescribeParamGroupsResp, err error) {
	params := makeParams("DescribeDBParameterGroups")
	addParamsList(params, "DBParameterGroupName", groupNames)
	filter.addParams(params)
	resp = &RDSDescribeParamGroupsResp{}
	err = RDS.query(params, resp)
	if err != nil {
		return nil, err
	}
	return
}

func (RDS *RDS) DescribeDBParameters(groupname []string, filter *Filter) (resp *RDSDescribeParamsResp, err error) {
	params := makeParams("DescribeDBParameters")
	addParamsList(params, "DBParameterGroupName", groupname)
	filter.addParams(params)
	resp = &RDSDescribeParamsResp{}
	err = RDS.query(params, resp)
	if err != nil {
		return nil, err
	}
	return
}

// ----------------------------------------------------------------------------
// Filtering helper.

// Filter builds filtering parameters to be used in an EC2 query which supports
// filtering.  For example:
//
//     filter := NewFilter()
//     filter.Add("architecture", "i386")
//     filter.Add("launch-index", "0")
//     resp, err := ec2.Instances(nil, filter)
//
type Filter struct {
	m map[string][]string
}

// NewFilter creates a new Filter.
func NewFilter() *Filter {
	return &Filter{make(map[string][]string)}
}

// Add appends a filtering parameter with the given name and value(s).
func (f *Filter) Add(name string, value ...string) {
	f.m[name] = append(f.m[name], value...)
}

func (f *Filter) addParams(params map[string]string) {
	if f != nil {
		a := make([]string, len(f.m))
		i := 0
		for k := range f.m {
			a[i] = k
			i++
		}
		sort.StringSlice(a).Sort()
		for i, k := range a {
			prefix := "Filter." + strconv.Itoa(i+1)
			params[prefix+".Name"] = k
			for j, v := range f.m[k] {
				params[prefix+".Value."+strconv.Itoa(j+1)] = v
			}
		}
	}
}

// ----------------------------------------------------------------------------
// Request dispatching logic.

// Error encapsulates an error returned by RDS.
//
// See http://goo.gl/VZGuC for more details.
type Error struct {
	// HTTP status code (200, 403, ...)
	StatusCode int
	// EC2 error code ("UnsupportedOperation", ...)
	Code string
	// The human-oriented error message
	Message   string
	RequestId string `xml:"RequestID"`
}

func (err *Error) Error() string {
	if err.Code == "" {
		return err.Message
	}

	return fmt.Sprintf("%s (%s)", err.Message, err.Code)
}

// For now a single error inst is being exposed. In the future it may be useful
// to provide access to all of them, but rather than doing it as an array/slice,
// use a *next pointer, so that it's backward compatible and it continues to be
// easy to handle the first error, which is what most people will want.
type xmlErrors struct {
	RequestId string  `xml:"RequestID"`
	Errors    []Error `xml:"Errors>Error"`
}

var timeNow = time.Now

func (rds *RDS) query(params map[string]string, resp interface{}) error {
	params["Version"] = "2013-05-15"
	params["Timestamp"] = timeNow().In(time.UTC).Format(time.RFC3339)
	endpoint, err := url.Parse(rds.Region.RDSEndpoint)
	if err != nil {
		return err
	}
	if endpoint.Path == "" {
		endpoint.Path = "/"
	}
	sign(rds.Auth, "GET", endpoint.Path, params, endpoint.Host)
	endpoint.RawQuery = multimap(params).Encode()
	if debug {
		log.Printf("get { %v } -> {\n", endpoint.String())
	}
	r, err := http.Get(endpoint.String())
	if err != nil {
		return err
	}
	defer r.Body.Close()

	if debug {
		dump, _ := httputil.DumpResponse(r, true)
		log.Printf("response:\n")
		log.Printf("%v\n}\n", string(dump))
	}
	if r.StatusCode != 200 {
		return buildError(r)
	}
	err = xml.NewDecoder(r.Body).Decode(resp)
	return err
}

func multimap(p map[string]string) url.Values {
	q := make(url.Values, len(p))
	for k, v := range p {
		q[k] = []string{v}
	}
	return q
}

func buildError(r *http.Response) error {
	errors := xmlErrors{}
	xml.NewDecoder(r.Body).Decode(&errors)
	var err Error
	if len(errors.Errors) > 0 {
		err = errors.Errors[0]
	}
	err.RequestId = errors.RequestId
	err.StatusCode = r.StatusCode
	if err.Message == "" {
		err.Message = r.Status
	}
	return &err
}

func makeParams(action string) map[string]string {
	params := make(map[string]string)
	params["Action"] = action
	return params
}

func addParamsList(params map[string]string, label string, ids []string) {
	for i, id := range ids {
		params[label+"."+strconv.Itoa(i+1)] = id
	}
}
