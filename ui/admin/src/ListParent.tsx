import React from 'react';
import { useOutlet, Outlet } from 'react-router-dom';

/**
 * List parent is a component to use to wrap a page that list results, but the individual
 * entries to that list will have child routes under the parent. This allows the parent to
 * render the list and the child routes to render the individual entries.
 */
export default function ListParent({children}: {children: React.ReactNode}): React.ReactNode {
    const outlet = useOutlet();
    const hasOutletContent = outlet !== null;

    let body: React.ReactNode;
    if (hasOutletContent) {
        body = <Outlet />;
    } else {
        body = children;
    }

    return body;
}