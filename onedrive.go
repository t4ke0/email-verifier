package emailverifier

import (
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"strings"
)

var baseURL string = "https://autodiscover-s.outlook.com/autodiscover/autodiscover.svc"

var headers = map[string][]string{
	"Content-Type":    []string{"text/xml; charset=utf-8"},
	"SOAPAction":      []string{`"http://schemas.microsoft.com/exchange/2010/Autodiscover/Autodiscover/GetFederationInformation"`},
	"User-Agent":      []string{"AutodiscoverClient"},
	"Accept-Encoding": []string{"identity"},
}

type Envelope struct {
	XMLName xml.Name `xml:"Envelope"`
	Body    Body     `xml:"Body"`
}

type Body struct {
	GetFederationInformationResponseMessage GetFederationInformationResponseMessage `xml:"GetFederationInformationResponseMessage"`
}

type GetFederationInformationResponseMessage struct {
	Response Response `xml:"Response"`
}

type Response struct {
	Domains Domains `xml:"Domains"`
}

type Domains struct {
	XMLName xml.Name `xml:"Domains"`
	Domain  []string `xml:"Domain"`
}

func OneDriveValidate(email string) (bool, error) {
	var (
		user       string
		domainName string
	)

	sp := strings.Split(email, "@")
	if len(sp) < 2 {
		return false, nil
	}

	user = sp[0]
	domainName = sp[1]

	var data string = `<?xml version="1.0" encoding="utf-8"?>
        <soap:Envelope xmlns:exm="http://schemas.microsoft.com/exchange/services/2006/messages" xmlns:ext="http://schemas.microsoft.com/exchange/services/2006/types" xmlns:a="http://www.w3.org/2005/08/addressing" xmlns:soap="http://schemas.xmlsoap.org/soap/envelope/" xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance" xmlns:xsd="http://www.w3.org/2001/XMLSchema">
            <soap:Header>
                <a:Action soap:mustUnderstand="1">http://schemas.microsoft.com/exchange/2010/Autodiscover/Autodiscover/GetFederationInformation</a:Action>
                <a:To soap:mustUnderstand="1">https://autodiscover-s.outlook.com/autodiscover/autodiscover.svc</a:To>
                <a:ReplyTo>
                    <a:Address>http://www.w3.org/2005/08/addressing/anonymous</a:Address>
                </a:ReplyTo>
            </soap:Header>
            <soap:Body>
                <GetFederationInformationRequestMessage xmlns="http://schemas.microsoft.com/exchange/2010/Autodiscover">
                    <Request>
                        <Domain>%s</Domain>
                    </Request>
                </GetFederationInformationRequestMessage>
            </soap:Body>
        </soap:Envelope>
`

	req, err := http.NewRequest(http.MethodPost, baseURL, strings.NewReader(fmt.Sprintf(data, domainName)))
	if err != nil {
		return false, err
	}

	req.Header = http.Header(headers)

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return false, err
	}

	defer resp.Body.Close()
	odata, err := io.ReadAll(resp.Body)
	if err != nil {
		return false, err
	}

	var envelope Envelope

	if err := xml.Unmarshal(odata, &envelope); err != nil {
		return false, err
	}

	var targetTenant string
	for _, domain := range envelope.Body.GetFederationInformationResponseMessage.Response.Domains.Domain {
		if !strings.HasSuffix(domain, "onmicrosoft.com") {
			continue
		}
		targetTenant = strings.Split(domain, ".")[0]
	}

	if targetTenant == "" {
		return false, nil
	}

	user = strings.ReplaceAll(user, ".", "_")
	domainName = strings.ReplaceAll(domainName, ".", "_")

	testURL := "https://" + targetTenant + "-my.sharepoint.com/personal/" + user + "_" + domainName + "/_layouts/15/onedrive.aspx"

	response, err := http.Get(testURL)
	if err != nil {
		return false, err
	}

	defer response.Body.Close()

	return (response.StatusCode == 200 || response.StatusCode == 401 || response.StatusCode == 403 || response.StatusCode == 302), nil
}

// func main() {
//
// 	scanner := bufio.NewScanner(os.Stdin)
// 	fmt.Printf("Enter Email address: ")
// 	scanner.Scan()
//
// 	email := scanner.Text()
//
// 	fmt.Println("Valid", Validate(email))
// }
