import * as React from 'react';
import {styled} from '@mui/material/styles';
import Typography from '@mui/material/Typography';
import Breadcrumbs, {breadcrumbsClasses} from '@mui/material/Breadcrumbs';
import NavigateNextRoundedIcon from '@mui/icons-material/NavigateNextRounded';
import {UIMatch, useMatches} from 'react-router-dom';


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
            path: match.pathname
        }));


    return (
        <StyledBreadcrumbs
            aria-label="breadcrumb"
            separator={<NavigateNextRoundedIcon fontSize="small"/>}
        >
            {crumbs.map((crumb, index) => {
                if (index === crumbs.length - 1) {
                    return (
                        <Typography variant="body1" sx={{color: 'text.primary', fontWeight: 600}}>
                            {crumb.title}
                        </Typography>
                    );
                }

                return (
                    <Typography variant="body1">
                        {crumb.title}
                    </Typography>
                );
            })}
        </StyledBreadcrumbs>
    );
}
