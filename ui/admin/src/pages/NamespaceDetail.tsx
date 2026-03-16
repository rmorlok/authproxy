import React from 'react';
import Box from '@mui/material/Box';
import {useSelector} from 'react-redux';
import {selectCurrentNamespacePath} from '../store/namespacesSlice';
import NamespaceDetailComponent from '../components/NamespaceDetail';

export default function NamespaceDetail() {
  const path = useSelector(selectCurrentNamespacePath);
  return (
    <Box sx={{width: '100%', maxWidth: {sm: '100%', md: '1700px'}}}>
      <NamespaceDetailComponent namespacePath={path} />
    </Box>
  );
}
