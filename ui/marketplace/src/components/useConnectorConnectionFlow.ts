import { useCallback } from 'react';
import { useDispatch, useSelector } from 'react-redux';
import { isRedirectResponse } from '@authproxy/api';
import {
  abortConnectionAsync,
  clearFormStep,
  initiateConnectionAsync,
  selectCurrentFormStep,
  selectFormSubmitError,
  selectInitiatingConnection,
  selectSubmittingForm,
  submitConnectionFormAsync,
} from '../store';
import { AppDispatch } from '../store';

export function useConnectorConnectionFlow() {
  const dispatch = useDispatch<AppDispatch>();
  const isConnecting = useSelector(selectInitiatingConnection);
  const currentFormStep = useSelector(selectCurrentFormStep);
  const isSubmittingForm = useSelector(selectSubmittingForm);
  const formSubmitError = useSelector(selectFormSubmitError);

  const returnToUrl = useCallback(() => `${window.location.origin}/connections`, []);

  const connect = useCallback((connectorId: string) => {
    dispatch(initiateConnectionAsync({
      connectorId,
      returnToUrl: returnToUrl(),
    })).then((action) => {
      if (action.meta.requestStatus === 'fulfilled') {
        const response = action.payload as any;
        if (isRedirectResponse(response)) {
          window.location.href = response.redirect_url;
        }
      }
    });
  }, [dispatch, returnToUrl]);

  const submitForm = useCallback((connectionId: string, data: unknown) => {
    const stepId = currentFormStep?.stepId ?? '';
    dispatch(submitConnectionFormAsync({
      connectionId,
      stepId,
      data,
      returnToUrl: returnToUrl(),
    })).then((action) => {
      if (action.meta.requestStatus === 'fulfilled') {
        const response = action.payload as any;
        if (isRedirectResponse(response)) {
          window.location.href = response.redirect_url;
        }
      }
    });
  }, [dispatch, currentFormStep, returnToUrl]);

  const cancelForm = useCallback(() => {
    if (currentFormStep) {
      dispatch(abortConnectionAsync(currentFormStep.connectionId));
    } else {
      dispatch(clearFormStep());
    }
  }, [dispatch, currentFormStep]);

  return {
    connect,
    currentFormStep,
    formSubmitError,
    isConnecting,
    isSubmittingForm,
    submitForm,
    cancelForm,
  };
}
