package chartserver

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/Masterminds/semver"

	hlog "github.com/vmware/harbor/src/common/utils/log"
	"k8s.io/helm/pkg/chartutil"
	helm_repo "k8s.io/helm/pkg/repo"
)

const (
	readmeFileName = "README.md"
)

//ChartVersionDetails keeps the detailed data info of the chart version
type ChartVersionDetails struct {
	Metadata     *helm_repo.ChartVersion `json:"metadata"`
	Dependencies []*chartutil.Dependency `json:"dependencies"`
	Values       map[string]interface{}  `json:"values"`
	Files        map[string]string       `json:"files"`
}

//ChartInfo keeps the information of the chart
type ChartInfo struct {
	Name          string
	TotalVersions uint32 `json:"total_versions"`
	Created       time.Time
	Icon          string
	Home          string
}

//ChartOperator is designed to process the contents of
//the specified chart version to get more details
type ChartOperator struct{}

//GetChartDetails parse the details from the provided content bytes
func (cho *ChartOperator) GetChartDetails(content []byte) (*ChartVersionDetails, error) {
	if content == nil || len(content) == 0 {
		return nil, errors.New("zero content")
	}

	//Load chart from in-memory content
	reader := bytes.NewReader(content)
	chartData, err := chartutil.LoadArchive(reader)
	if err != nil {
		return nil, err
	}

	//Parse the requirements of chart
	requirements, err := chartutil.LoadRequirements(chartData)
	if err != nil {
		return nil, err
	}

	var values map[string]interface{}
	files := make(map[string]string)
	//Parse values
	if chartData.Values != nil {
		values = parseRawValues([]byte(chartData.Values.GetRaw()))
		//Append values.yaml file
		files["values.yaml"] = chartData.Values.Raw
	}

	//Append other files like 'README.md'
	for _, v := range chartData.GetFiles() {
		if v.TypeUrl == readmeFileName {
			files[readmeFileName] = string(v.GetValue())
			break
		}
	}

	theChart := &ChartVersionDetails{
		Dependencies: requirements.Dependencies,
		Values:       values,
		Files:        files,
	}

	return theChart, nil
}

//GetChartList returns a reorganized chart list
func (cho *ChartOperator) GetChartList(content []byte) ([]*ChartInfo, error) {
	if content == nil || len(content) == 0 {
		return nil, errors.New("zero content")
	}

	allCharts := make(map[string]helm_repo.ChartVersions)
	if err := json.Unmarshal(content, &allCharts); err != nil {
		return nil, err
	}

	chartList := make([]*ChartInfo, 0)
	for key, chartVersions := range allCharts {
		lVersion, oVersion := getTheTwoCharts(chartVersions)
		if lVersion != nil && oVersion != nil {
			chartInfo := &ChartInfo{
				Name:          key,
				TotalVersions: uint32(len(chartVersions)),
			}
			chartInfo.Created = oVersion.Created
			chartInfo.Home = lVersion.Home
			chartInfo.Icon = lVersion.Icon
			chartList = append(chartList, chartInfo)
		}
	}

	return chartList, nil
}

//Get the latest and oldest chart versions
func getTheTwoCharts(chartVersions helm_repo.ChartVersions) (latestChart *helm_repo.ChartVersion, oldestChart *helm_repo.ChartVersion) {
	if len(chartVersions) == 1 {
		return chartVersions[0], chartVersions[0]
	}

	for _, chartVersion := range chartVersions {
		currentV, err := semver.NewVersion(chartVersion.Version)
		if err != nil {
			//ignore it, just logged
			hlog.Warningf("Malformed semversion %s for the chart %s", chartVersion.Version, chartVersion.Name)
			continue
		}

		//Find latest chart
		if latestChart == nil {
			latestChart = chartVersion
		} else {
			lVersion, _ := semver.NewVersion(latestChart.Version)
			if lVersion.LessThan(currentV) {
				latestChart = chartVersion
			}
		}

		if oldestChart == nil {
			oldestChart = chartVersion
		} else {
			if oldestChart.Created.After(chartVersion.Created) {
				oldestChart = chartVersion
			}
		}
	}

	return latestChart, oldestChart
}

//Parse the raw values to value map
func parseRawValues(rawValue []byte) map[string]interface{} {
	valueMap := make(map[string]interface{})

	if len(rawValue) == 0 {
		return valueMap
	}

	values, err := chartutil.ReadValues(rawValue)
	if err != nil || len(values) == 0 {
		return valueMap
	}

	readValue(values, "", valueMap)

	return valueMap
}

//Recursively read value
func readValue(values map[string]interface{}, keyPrefix string, valueMap map[string]interface{}) {
	for key, value := range values {
		longKey := key
		if keyPrefix != "" {
			longKey = fmt.Sprintf("%s.%s", keyPrefix, key)
		}

		if subValues, ok := value.(map[string]interface{}); ok {
			readValue(subValues, longKey, valueMap)
		} else {
			valueMap[longKey] = value
		}
	}
}
