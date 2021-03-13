import React, {FC} from "react";
import {ButtonGroup} from "@components/ButtonGroup/ButtonGroup";

export enum GameTypes {
    PlayOnline,
    PlayAFriend,
    PlayComputer
}

interface GameOptionsProps {
    gameType: GameTypes;
}

export const GameOptions: FC<GameOptionsProps> = (props) => {
    return (
            <div className="mt-3 text-center sm:mt-0 sm:text-left w-full">
                <h3 className="text-2xl leading-6 font-medium text-gray-900 text-center" id="modal-headline">
                    Game Options
                </h3>
                <div className="mt-4 flex flex-col justify-center items-center">

                    {/* Game type */}
                    {props.gameType === GameTypes.PlayOnline ?
                        <ButtonGroup>
                            <ButtonGroup.Button
                                onClick={() => console.log("1")}
                            >
                                Rated
                            </ButtonGroup.Button>
                            <ButtonGroup.Button
                                onClick={() => console.log("2")}
                            >
                                Casual
                            </ButtonGroup.Button>
                        </ButtonGroup> : null}

                    {/* Time controls + difficulty */}

                    {/* Color selection */}

                    {/* Action button */}
                </div>
            </div>
    )
}