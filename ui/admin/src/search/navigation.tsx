import type {SvgIconComponent} from '@mui/icons-material';
import HomeRoundedIcon from '@mui/icons-material/HomeRounded';
import PowerRoundedIcon from '@mui/icons-material/Power';
import LinkRoundedIcon from '@mui/icons-material/Link';
import HttpRoundedIcon from '@mui/icons-material/Http';
import PeopleRoundedIcon from '@mui/icons-material/PeopleRounded';
import AssignmentRoundedIcon from '@mui/icons-material/AssignmentRounded';
import KeyRoundedIcon from '@mui/icons-material/KeyRounded';
import SpeedRoundedIcon from '@mui/icons-material/SpeedRounded';
import AccountTreeRoundedIcon from '@mui/icons-material/AccountTreeRounded';
import InfoRoundedIcon from '@mui/icons-material/InfoRounded';
import HelpRoundedIcon from '@mui/icons-material/HelpRounded';
import SettingsRoundedIcon from '@mui/icons-material/SettingsRounded';

export interface AdminNavigationItem {
    label: string;
    icon: SvgIconComponent;
    path: string;
    keywords: string[];
    section: 'main' | 'secondary';
    external?: boolean;
    searchable?: boolean;
}

export const adminNavigationItems: AdminNavigationItem[] = [
    {label: 'Home', icon: HomeRoundedIcon, path: '/home', keywords: ['dashboard'], section: 'main'},
    {label: 'Namespace', icon: AccountTreeRoundedIcon, path: '/namespace', keywords: ['tenant'], section: 'main'},
    {label: 'Connectors', icon: PowerRoundedIcon, path: '/connectors', keywords: ['integrations'], section: 'main'},
    {label: 'Connections', icon: LinkRoundedIcon, path: '/connections', keywords: ['accounts'], section: 'main'},
    {label: 'Requests', icon: HttpRoundedIcon, path: '/requests', keywords: ['events', 'logs'], section: 'main'},
    {label: 'Tasks', icon: AssignmentRoundedIcon, path: '/tasks', keywords: ['queues', 'jobs'], section: 'main'},
    {label: 'Workflows', icon: AccountTreeRoundedIcon, path: '/workflows', keywords: ['executions'], section: 'main'},
    {label: 'Keys', icon: KeyRoundedIcon, path: '/keys', keywords: ['encryption'], section: 'main'},
    {label: 'Rate Limits', icon: SpeedRoundedIcon, path: '/rate-limits', keywords: ['quota'], section: 'main'},
    {label: 'Actors', icon: PeopleRoundedIcon, path: '/actors', keywords: ['users'], section: 'main'},
    {
        label: 'Settings',
        icon: SettingsRoundedIcon,
        path: '/settings',
        keywords: [],
        section: 'secondary',
        searchable: false,
    },
    {label: 'About', icon: InfoRoundedIcon, path: '/about', keywords: ['version'], section: 'secondary'},
    {
        label: 'Feedback',
        icon: HelpRoundedIcon,
        path: 'https://github.com/rmorlok/authproxy/issues',
        keywords: ['github', 'help'],
        section: 'secondary',
        external: true,
    },
];

export function matchingNavigationItems(text: string): AdminNavigationItem[] {
    const needle = text.trim().toLowerCase();
    const searchableItems = adminNavigationItems.filter((item) => item.searchable !== false);
    if (!needle) return searchableItems;
    return searchableItems.filter((item) =>
        [item.label, ...item.keywords].some((value) => value.toLowerCase().includes(needle)),
    );
}
