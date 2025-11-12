import * as React from 'react';
import {styled} from '@mui/material/styles';
import Typography from '@mui/material/Typography';
import Breadcrumbs, {breadcrumbsClasses} from '@mui/material/Breadcrumbs';
import NavigateNextRoundedIcon from '@mui/icons-material/NavigateNextRounded';
import {Link, UIMatch, useMatches} from 'react-router-dom';


const StyledBreadcrumbs = styled(Breadcrumbs)(({theme}) => ({
    margin: theme.spacing(1, 0),
    [`& .${breadcrumbsClasses.separator}`]: {
        color: (theme.vars || theme).palette.action.disabled,
        margin: 1,
    },
    [`& .${breadcrumbsClasses.ol}`]: {
        alignItems: 'center',
    },
}));

interface RouteHandle {
    title?: string
    attr?: string
}

export default function NavbarBreadcrumbs() {
    const matches = useMatches() as UIMatch<unknown, RouteHandle | RouteHandle[]>[];
    const crumbs = matches
        .flatMap((match) => {
            const handles = !match?.handle ? [] : (Array.isArray(match?.handle) ? match.handle : [match.handle]);
            return handles
                .filter((match) => match?.title || match?.attr)
                .map((handle) => {
                if (handle?.title) {
                    return {
                        title: handle?.title,
                        path: match.pathname,
                    };
                }

                if (handle?.attr) {
                    return {
                        title: match.params[handle?.attr] || `<UNKNOWN PARAM ${handle?.attr}>`,
                        path: match.pathname,
                    }
                }

                return {
                    title: '<UNKNOWN>',
                    path: match.pathname,
                }
            });
        })

    return (
        <StyledBreadcrumbs
            aria-label="breadcrumb"
            separator={<NavigateNextRoundedIcon fontSize="small"/>}
        >
            {crumbs.map((crumb, index) => {
                if (index === crumbs.length - 1) {
                    return (
                        <Typography key={index} variant="body1" sx={{color: 'text.primary', fontWeight: 600}}>
                            {crumb.title}
                        </Typography>
                    );
                }

                return (
                    <Typography key={index} variant="body1" component={Link} to={crumb.path}
                                sx={{color: 'text.primary', textDecoration: 'none'}}>
                        {crumb.title}
                    </Typography>
                );
            })}
        </StyledBreadcrumbs>
    );
}
