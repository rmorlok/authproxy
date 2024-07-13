import {ApiUser} from "../api";
import {Player} from "../api/models";

export const JOE_BIDEN: ApiUser = {
    name: "Joe Biden",
    id: "joe_biden_46",
    picture: "/src/stories/assets/presidents/joe_biden.jpeg",
    email: "joebiden@example.com",
};

export const DONALD_TRUMP: ApiUser = {
    name: "Donald Trump",
    id: "donald_trump_45",
    picture: "/src/stories/assets/presidents/donald_trump.jpeg",
    email: "donaldtrump@example.com",
};

export const BARACK_OBAMA: ApiUser = {
    name: "Barack Obama",
    id: "barack_obama_44",
    picture: "/src/stories/assets/presidents/barack_obama.jpeg",
    email: "barackobama@example.com",
};

export const GEORGE_W_BUSH: ApiUser = {
    name: "George W Bush",
    id: "george_w_bush_43",
    picture: "/src/stories/assets/presidents/george_w_bush.jpeg",
    email: "georgewbush@example.com",
};

export const BILL_CLINTON: ApiUser = {
    name: "Bill Clinton",
    id: "bill_clinton_42",
    picture: "/src/stories/assets/presidents/bill_clinton.jpeg",
    email: "billclinton@example.com",
};

export const GEORGE_HW_BUSH: ApiUser = {
    name: "George HW Bush",
    id: "george_hw_bush_41",
    picture: "/src/stories/assets/presidents/george_hw_bush.jpeg",
    email: "georgehwbush@example.com",
};

export const RONALD_REAGAN: ApiUser = {
    name: "Ronald Reagan",
    id: "ronald_reagan_40",
    picture: "/src/stories/assets/presidents/ronald_reagan.jpeg",
    email: "ronaldreagan@example.com",
};

export const JIMMY_CARTER: ApiUser = {
    name: "Jimmy Carter",
    id: "jimmy_carter_39",
    picture: "/src/stories/assets/presidents/jimmy_carter.jpeg",
    email: "jimmycarter@example.com",
};

export const BOB_DOLE: ApiUser = {
    name: "Bob Dole",
    id: "bob_dole_xx",
    picture: "/src/stories/assets/presidents/bob_dole.jpeg",
    email: "bobdole@example.com",
};

export const FAKE_USERS: ApiUser[] = [
  BOB_DOLE,
  JIMMY_CARTER,
  RONALD_REAGAN,
  GEORGE_HW_BUSH,
  BILL_CLINTON,
  GEORGE_W_BUSH,
  BARACK_OBAMA,
  DONALD_TRUMP,
  JOE_BIDEN,
];

export const isBobDole = (u: ApiUser | Player): boolean => {
    if('user' in u) {
        // Player; recurse
        return isBobDole(u.user);
    }

    return u.id === 'bob_dole_xx';
}

export function generateFakePlayers() {
    let alternate = 0;
    const fake_players: Player[] = FAKE_USERS.map((u) => {
        if(isBobDole(u)) {
            return {
                user: u,
                isSpyMaster: false,
                team: 'undecided',
            }
        }

        return {
            user: u,
            isSpyMaster: alternate === 0,
            team: alternate++ % 2 === 0 ? 'red' : 'blue',
        }
    });
    const this_player = fake_players.filter(isBobDole)[0];
    return {fake_players, this_player};
}
