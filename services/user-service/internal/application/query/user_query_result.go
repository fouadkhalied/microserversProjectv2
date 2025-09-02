package query

import "user-service-new/internal/application/common"

type UserQueryResult struct {
	Result *common.UserResult `json:"result"`
}

type UserQueryListResult struct {
	Result []*common.UserResult `json:"result"`
}
