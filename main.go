/*
 * This file is subject to the terms and conditions defined in
 * file 'LICENSE', which is part of this source code package.
 */

package main

import (
	"encoding/json"
	"encoding/xml"
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

type WorkflowsRsp struct {
	Workflows tukdbint.Workflows
	State     tukxdw.XDWState
	Dashboard tukxdw.Dashboard
}
type WorkflowRsp struct {
	Workflow tukxdw.XDWWorkflowDocument
}
type WorkflowsStateRsp struct {
	Dashboard     tukxdw.Dashboard
	WorkflowState []WorkflowState
}
type WorkflowState struct {
	Workflow string
	XDWState tukxdw.XDWState
}

func main() {
	lambda.Start(Handle_Request)
}
func Handle_Request(req events.APIGatewayProxyRequest) (*events.APIGatewayProxyResponse, error) {
	log.SetFlags(log.Lshortfile)

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
	contentType := tukcnst.TEXT_PLAIN
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
		case tukcnst.TUK_EVENT_QUERY_PARAM_FORMAT:
			contentType = value
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
				return queryResponse(http.StatusOK, trans.XDWState.Status, contentType)
			case "duration":
				return queryResponse(http.StatusOK, trans.XDWState.PrettyWorkflowDuration, contentType)
			case "isoverdue":
				return queryResponse(http.StatusOK, strconv.FormatBool(trans.XDWState.IsOverdue), contentType)
			case "created":
				return queryResponse(http.StatusOK, trans.XDWState.Created, contentType)
			case "completeby":
				return queryResponse(http.StatusOK, trans.XDWState.CompleteBy, contentType)
			case "updated":
				return queryResponse(http.StatusOK, trans.XDWState.LatestWorkflowEventTime.String(), contentType)
			case "count":
				return queryResponse(http.StatusOK, wfcount, contentType)
			case "states":
				return queryResponse(http.StatusOK, setWorkflowsStates(trans.Dashboard, trans.Workflows), tukcnst.APPLICATION_JSON)
			}
		}
		contentType = tukcnst.APPLICATION_JSON
		var wfs []byte
		if trans.Workflows.Count == 1 {
			rsp := WorkflowRsp{}
			xml.Unmarshal([]byte(trans.Workflows.Workflows[1].XDW_Doc), &rsp.Workflow)
			if wfs, err = json.MarshalIndent(rsp, "", "  "); err == nil {
				return queryResponse(http.StatusOK, string(wfs), contentType)
			}
		} else {
			rsp := WorkflowsRsp{
				Workflows: trans.Workflows,
				State:     trans.XDWState,
				Dashboard: trans.Dashboard,
			}
			if wfs, err = json.MarshalIndent(rsp, "", "  "); err == nil {
				return queryResponse(http.StatusOK, string(wfs), contentType)
			}
		}
	}
	return queryResponse(http.StatusInternalServerError, err.Error(), contentType)
}
func setWorkflowsStates(db tukxdw.Dashboard, wfs tukdbint.Workflows) string {
	var wfstates []byte
	var err error
	rsp := WorkflowsStateRsp{Dashboard: db}
	for _, wf := range wfs.Workflows {
		trans := tukxdw.Transaction{}
		trans.Actor = tukcnst.XDW_ACTOR_CONTENT_CONSUMER
		trans.Pathway = wf.Pathway
		trans.NHS_ID = wf.NHSId
		trans.XDWVersion = wf.Version
		if err := tukxdw.Execute(&trans); err != nil {
			log.Println(err.Error())
			return err.Error()
		}
		if trans.Workflows.Count == 1 {
			wfState := WorkflowState{Workflow: wf.Pathway, XDWState: trans.XDWState}
			rsp.WorkflowState = append(rsp.WorkflowState, wfState)
		} else {
			log.Printf("Duplicate workflows found for Workflow=%s NHS=%s Vers=%v, skipping.....", trans.Pathway, trans.NHS_ID, trans.XDWVersion)
		}

	}
	if wfstates, err = json.MarshalIndent(rsp, "", "  "); err != nil {
		log.Println(err.Error())
		return err.Error()
	}
	return string(wfstates)
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
