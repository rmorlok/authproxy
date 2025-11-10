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

export default function NavbarBreadcrumbs() {
    const matches = useMatches() as UIMatch<unknown, { title?: string }>[];
    const crumbs = matches
        .filter((match) => match.handle && match.handle.title)
        .map((match) => ({
            title: match.handle.title,
            path: match.pathname,
            params: (match as any).params as Record<string, string | undefined>,
        }));

    // If the last match has an id param, append it as a third-level breadcrumb
    let finalCrumbs = crumbs as Array<{ title: string; path: string; params?: Record<string, string | undefined> }>;
    const last = finalCrumbs[finalCrumbs.length - 1];
    const idParam = last?.params?.id;
    if (idParam) {
        finalCrumbs = [...finalCrumbs, { title: idParam, path: `${last.path}` }];
    }

    return (
        <StyledBreadcrumbs
            aria-label="breadcrumb"
            separator={<NavigateNextRoundedIcon fontSize="small"/>}
        >
            {finalCrumbs.map((crumb, index) => {
                if (index === finalCrumbs.length - 1) {
                    return (
                        <Typography key={index} variant="body1" sx={{color: 'text.primary', fontWeight: 600}}>
                            {crumb.title}
                        </Typography>
                    );
                }

                return (
                    <Typography key={index} variant="body1" component={Link} to={crumb.path} sx={{color: 'text.primary', textDecoration: 'none'}}>
                        {crumb.title}
                    </Typography>
                );
            })}
        </StyledBreadcrumbs>
    );
}
