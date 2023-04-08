/*
 * This file is subject to the terms and conditions defined in
 * file 'LICENSE', which is part of this source code package.
 */

package main

import (
	"encoding/json"
	"log"
	"net/http"
	"os"
	"strconv"

	"github.com/ipthomas/tukcnst"
	"github.com/ipthomas/tukdbint"
	"github.com/ipthomas/tukutil"
	"github.com/ipthomas/tukxdw"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambda"
)

var initSrvcs = false

type XDW_Consumer_Rsp struct {
	Workflows tukdbint.Workflows
	State     tukxdw.XDWState
	Dashboard tukxdw.Dashboard
}

func main() {
	lambda.Start(Handle_Request)
}
func Handle_Request(req events.APIGatewayProxyRequest) (*events.APIGatewayProxyResponse, error) {
	var err error
	if !initSrvcs {
		dbconn := tukdbint.TukDBConnection{DBUser: os.Getenv(tukcnst.ENV_DB_USER), DBPassword: os.Getenv(tukcnst.ENV_DB_PASSWORD), DBHost: os.Getenv(tukcnst.ENV_DB_HOST), DBPort: os.Getenv(tukcnst.ENV_DB_PORT), DBName: os.Getenv(tukcnst.ENV_DB_NAME)}
		if err = tukdbint.NewDBEvent(&dbconn); err != nil {
			return queryResponse(http.StatusInternalServerError, err.Error(), tukcnst.TEXT_PLAIN)
		}
		initSrvcs = true
	}

	log.Printf("Processing API Gateway %s Request Path %s", req.HTTPMethod, req.Path)
	trans := tukxdw.Transaction{Actor: tukcnst.XDW_ACTOR_CONTENT_CONSUMER, XDWVersion: -1}
	op := ""
	for key, value := range req.QueryStringParameters {
		log.Printf("    %s: %s\n", key, value)
		switch key {
		case tukcnst.TUK_EVENT_QUERY_PARAM_VERSION:
			if value != "" {
				trans.XDWVersion = tukutil.GetIntFromString(value)
			}
		case tukcnst.TUK_EVENT_QUERY_PARAM_PATHWAY:
			trans.Pathway = value
		case tukcnst.TUK_EVENT_QUERY_PARAM_NHS:
			trans.NHS_ID = value
		case tukcnst.TUK_EVENT_QUERY_PARAM_OP:
			op = value
		}
	}
	if err = tukxdw.Execute(&trans); err == nil {
		if trans.Workflows.Count == 0 {
			return queryResponse(http.StatusOK, "0", tukcnst.TEXT_PLAIN)
		}
		if op != "" {
			wfcount := strconv.Itoa(trans.Workflows.Count)
			switch op {
			case "status":
				return queryResponse(http.StatusOK, trans.XDWState.Status, tukcnst.TEXT_PLAIN)
			case "duration":
				return queryResponse(http.StatusOK, trans.XDWState.PrettyWorkflowDuration, tukcnst.TEXT_PLAIN)
			case "isoverdue":
				return queryResponse(http.StatusOK, strconv.FormatBool(trans.XDWState.IsOverdue), tukcnst.TEXT_PLAIN)
			case "created":
				return queryResponse(http.StatusOK, trans.XDWState.Created, tukcnst.TEXT_PLAIN)
			case "completeby":
				return queryResponse(http.StatusOK, trans.XDWState.CompleteBy, tukcnst.TEXT_PLAIN)
			case "updated":
				return queryResponse(http.StatusOK, trans.XDWState.LatestWorkflowEventTime.String(), tukcnst.TEXT_PLAIN)
			case "count":
				return queryResponse(http.StatusOK, wfcount, tukcnst.TEXT_PLAIN)
			}
		}
		rsp := XDW_Consumer_Rsp{
			Workflows: trans.Workflows,
			State:     trans.XDWState,
			Dashboard: trans.Dashboard,
		}
		var wfs []byte
		if wfs, err = json.MarshalIndent(rsp, "", "  "); err == nil {
			return queryResponse(http.StatusOK, string(wfs), tukcnst.APPLICATION_JSON)
		}
	}
	return queryResponse(http.StatusInternalServerError, err.Error(), tukcnst.TEXT_PLAIN)
}
func setAwsResponseHeaders(contentType string) map[string]string {
	awsHeaders := make(map[string]string)
	awsHeaders["Server"] = "TUK_XDW_Consumer_Proxy"
	awsHeaders["Access-Control-Allow-Origin"] = "*"
	awsHeaders["Access-Control-Allow-Headers"] = "accept, Content-Type"
	awsHeaders["Access-Control-Allow-Methods"] = "GET, POST, OPTIONS"
	awsHeaders[tukcnst.CONTENT_TYPE] = contentType
	return awsHeaders
}
func queryResponse(statusCode int, body string, contentType string) (*events.APIGatewayProxyResponse, error) {
	log.Println(body)
	return &events.APIGatewayProxyResponse{
		StatusCode: statusCode,
		Headers:    setAwsResponseHeaders(contentType),
		Body:       body,
	}, nil
}
