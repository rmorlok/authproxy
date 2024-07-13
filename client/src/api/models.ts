import {ApiUser} from "./auth";

export type Team = 'red' | 'blue' | 'undecided';

export type Player = {
    team: Team;
    user: ApiUser;
    isSpyMaster: boolean;
};

export type CardColor = Team | 'yellow' | 'black';

export type WordCardModel = {
    word: string;
    revealed: boolean;
    color: CardColor;
}

export type BoardModel = WordCardModel[][];

/**
 * Reveals a card in such a way that the board is transformed to new copy with the board revealed. Intended
 * to be used with react state/redux/etc.
 *
 * @param board the board in play
 * @param word the word to be revealed.
 */
export const revealCard = (board: BoardModel, word: string): BoardModel => {
    return board.map((row) => {
        return row.map((card) => {
            if (card.word === word) {
                return {
                    ...card,
                    revealed: true,
                }
            } else {
                return {
                    ...card,
                }
            }
        });
    })
}