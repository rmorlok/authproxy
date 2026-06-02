import React from 'react';
import {
  Alert,
  AlertTitle,
  Box,
  Dialog,
  DialogContent,
  DialogTitle,
  Typography,
} from '@mui/material';
import ConnectionFormStep from './ConnectionFormStep';
import { marketplaceTokens } from '../theme';

interface SetupStep {
  connectionId: string;
  stepId: string;
  stepTitle?: string;
  stepDescription?: string;
  jsonSchema: Record<string, unknown>;
  uiSchema: Record<string, unknown>;
}

interface ConnectionSetupDialogProps {
  currentFormStep: SetupStep | null;
  formSubmitError: string | null;
  isSubmittingForm: boolean;
  onCancel: () => void;
  onSubmit: (connectionId: string, data: unknown) => void;
}

const ConnectionSetupDialog: React.FC<ConnectionSetupDialogProps> = ({
  currentFormStep,
  formSubmitError,
  isSubmittingForm,
  onCancel,
  onSubmit,
}) => {
  return (
    <Dialog
      open={currentFormStep !== null}
      onClose={onCancel}
      maxWidth="sm"
      fullWidth
    >
      <DialogTitle sx={{ pb: 1 }}>
        <Box sx={{ display: 'flex', flexDirection: 'column', gap: marketplaceTokens.spacing.cardActionGap }}>
          <Typography variant="h6" component="span">
            Complete setup
          </Typography>
          <Typography variant="body2" color="text.secondary">
            {currentFormStep?.stepTitle ?? 'Provide the details needed to finish this connection.'}
          </Typography>
        </Box>
      </DialogTitle>
      <DialogContent dividers>
        {formSubmitError && (
          <Alert severity="error" sx={{ mb: 2 }}>
            <AlertTitle>Setup could not be saved</AlertTitle>
            {formSubmitError}
          </Alert>
        )}
        {currentFormStep && (
          <ConnectionFormStep
            connectionId={currentFormStep.connectionId}
            stepTitle={currentFormStep.stepTitle}
            stepDescription={currentFormStep.stepDescription}
            jsonSchema={currentFormStep.jsonSchema}
            uiSchema={currentFormStep.uiSchema}
            onSubmit={onSubmit}
            onCancel={onCancel}
            isSubmitting={isSubmittingForm}
          />
        )}
      </DialogContent>
    </Dialog>
  );
};

export default ConnectionSetupDialog;
