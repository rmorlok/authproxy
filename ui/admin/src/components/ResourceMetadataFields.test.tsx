// @vitest-environment jsdom
import * as React from 'react';
import {afterEach, describe, expect, it} from 'vitest';
import {cleanup, render, screen, within} from '@testing-library/react';
import {ResourceLabels, ResourceNamespace} from './ResourceMetadataFields';

describe('ResourceMetadataFields', () => {
  afterEach(cleanup);

  it('renders namespaces consistently and sorts label chips by key', () => {
    render(
      <>
        <ResourceNamespace namespace="root.payments"/>
        <ResourceLabels labels={{zebra: 'last', alpha: 'first'}}/>
      </>,
    );

    const namespace = screen.getByText('Namespace').parentElement;
    expect(namespace).not.toBeNull();
    expect(within(namespace!).getByText('root.payments').tagName).toBe('CODE');

    const labels = screen.getByText('Labels').parentElement;
    expect(labels).not.toBeNull();
    const values = within(labels!).getAllByText(/^(alpha|zebra):/).map(element => element.textContent);
    expect(values).toEqual(['alpha: first', 'zebra: last']);
  });

  it('uses the same empty-state copy for every resource', () => {
    render(<ResourceLabels/>);
    expect(screen.getByText('No labels')).toBeTruthy();
  });
});
