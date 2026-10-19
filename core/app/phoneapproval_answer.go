package app

import (
	"context"
	"errors"
	"net/http"
	"sort"
	"time"

	"github.com/tryhisar/hisar/core/apiclient"
	"github.com/tryhisar/hisar/core/approval"
)

// The answering side of phone approvals: the iPhone lists the Mac's open requests and signs its answer.

// PhoneApprovalRequest is a Mac's request as the phone shows it; times are RFC 3339.
type PhoneApprovalRequest struct {
	RequestID           string `json:"requestId"`
	RequesterDeviceName string `json:"requesterDeviceName"`
	CreatedAt           string `json:"createdAt"`
	ExpiresAt           string `json:"expiresAt"`
	approval.Description
}

// ListPendingPhoneApprovals opens the other devices' open requests; expired, answered and withdrawn ones are left out.
func ListPendingPhoneApprovals(s *Service) ([]PhoneApprovalRequest, error) {
	apiClient, err := s.getUnlockedClient()
	if err != nil {
		return nil, err
	}
	ownDeviceID, err := s.getOwnDeviceID(apiClient)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), phoneApprovalRequestTimeout)
	defer cancel()
	pendingRequests, err := apiClient.ListPendingApprovals(ctx)
	if err != nil {
		return nil, convertError(err)
	}
	s.mutex.Lock()
	defer s.mutex.Unlock()
	if !s.isUnlocked() || s.unlockedSession.Client != apiClient {
		return nil, ErrLocked
	}
	requests := []PhoneApprovalRequest{}
	for _, pending := range pendingRequests {
		request, err := approval.OpenRequest(s.unlockedSession.Keyset, s.unlockedSession.AccountID, pending.RequestID, pending.SealedRequest, ownDeviceID, s.config.Now())
		if err != nil {
			continue
		}
		requests = append(requests, PhoneApprovalRequest{
			RequestID:           request.RequestID,
			RequesterDeviceName: request.RequesterDeviceName,
			CreatedAt:           request.CreatedAt.UTC().Format(time.RFC3339Nano),
			ExpiresAt:           request.ExpiresAt.UTC().Format(time.RFC3339Nano),
			Description:         request.Description,
		})
	}
	sort.SliceStable(requests, func(first, second int) bool { return requests[first].CreatedAt < requests[second].CreatedAt })
	return requests, nil
}

// AnswerPhoneApproval signs this device's decision and, for a request that offers it, the remember switch; the caller checked Face ID
// or the master password for an approval. Not a method, so the desktop UI cannot call it.
func AnswerPhoneApproval(s *Service, requestID string, isApproved, isRemembered bool) error {
	if !approval.IsValidRequestID(requestID) {
		return errPhoneApprovalGone
	}
	apiClient, err := s.getUnlockedClient()
	if err != nil {
		return err
	}
	ownDeviceID, err := s.getOwnDeviceID(apiClient)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), phoneApprovalRequestTimeout)
	defer cancel()
	stored, err := apiClient.GetApproval(ctx, requestID)
	if apiclient.IsStatus(err, http.StatusNotFound) {
		return errPhoneApprovalGone
	}
	if err != nil {
		return convertError(err)
	}
	if stored.SealedAnswer != nil || stored.IsResolved {
		return errPhoneApprovalGone
	}
	decision := approval.DecisionDeny
	if isApproved {
		decision = approval.DecisionApprove
	}
	s.mutex.Lock()
	if !s.isUnlocked() || s.unlockedSession.Client != apiClient {
		s.mutex.Unlock()
		return ErrLocked
	}
	s.noteActivity()
	now := s.config.Now()
	var sealedAnswer []byte
	request, err := approval.OpenRequest(s.unlockedSession.Keyset, s.unlockedSession.AccountID, requestID, stored.SealedRequest, ownDeviceID, now)
	if err == nil {
		sealedAnswer, err = approval.CreateRememberingAnswer(s.unlockedSession.Keyset, s.unlockedSession.AccountID, stored.SealedRequest, request, decision, isRemembered, ownDeviceID, s.config.DeviceName, now)
	}
	s.mutex.Unlock()
	if errors.Is(err, approval.ErrExpired) {
		return errPhoneApprovalGone
	}
	if err != nil {
		return err
	}
	err = apiClient.AnswerApproval(ctx, requestID, sealedAnswer)
	if apiclient.IsStatus(err, http.StatusNotFound) || apiclient.IsStatus(err, http.StatusConflict) || apiclient.IsStatus(err, http.StatusGone) {
		return errPhoneApprovalGone
	}
	return convertError(err)
}
