import React from 'react';
import Box from '@mui/material/Box';
import {useSelector} from 'react-redux';
import {selectCurrentNamespacePath} from '../store/namespacesSlice';
import NamespaceDetailComponent from '../components/NamespaceDetail';
import {Navigate, useParams} from 'react-router-dom';
import {namespaceDetailPath} from '../util';

export default function NamespaceDetail() {
  const currentPath = useSelector(selectCurrentNamespacePath);
  const {path} = useParams<{path: string}>();
  return (
    <Box sx={{width: '100%', maxWidth: {sm: '100%', md: '1700px'}}}>
      <NamespaceDetailComponent namespacePath={path || currentPath} />
    </Box>
  );
}

export function CurrentNamespaceDetailRedirect() {
  const currentPath = useSelector(selectCurrentNamespacePath);
  return <Navigate to={namespaceDetailPath(currentPath)} replace/>;
}
