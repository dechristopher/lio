import React, {FC, useEffect, useState} from "react";
import {ButtonGroup} from "@components/ButtonGroup/ButtonGroup";
import {Select} from "@components/Select/Select";
import WhitePiece from "@assets/images/pieces/White.svg";
import SplitPiece from "@assets/images/pieces/Split.svg";
import BlackPiece from "@assets/images/pieces/Black.svg";
import {bgColors} from "@utils/styles/colors";
import {Button} from "@components/Button/Button";

export enum GameModes {
    PlayOnline,
    PlayAFriend,
    PlayComputer
}

enum GameTypes {
    RATED = "Rated",
    CASUAL = "Casual"
}

enum GameTimes {
    Zero = "0:00",
    Five = "0:05",
    Fifteen = "0:15",
    Thirty = "0:30",
    OneMin = "1:00",
    ThreeMin = "3:00",
    FiveMin = "5:00",
    TenMin = "10:00"
}

enum GameIncrements {
    Zero = "0:00",
    One = "0:01",
    Three = "0:03",
    Five = "0:05",
    Ten = "0:10",
    Fifteen = "0:15",
    Thirty = "0:30",
}

enum GameDelays {
    Zero = "0:00",
    One = "0:01",
    Three = "0:03",
    Five = "0:05",
    Ten = "0:10",
    Fifteen = "0:15",
    Thirty = "0:30",
}

enum ComputerDifficulties {
    One = "1",
    Two = "2",
    Three = "3",
    Four = "4",
    Five = "5",
    Six = "6",
    Seven = "7",
    Eight = "8",
    Nine = "9",
    Ten = "10",
}

enum ColorOptions {
    White,
    Black,
    Random

}

enum ButtonActions {
    FindGame = "Find Game",
    StartGame = "Start Game"
}

const pieceButtonStyle = {
    padding: ".25rem .5rem"
}
const pieceStyle = {
    width: "3em",
    height: "3em"
};
const optionPadding = "pt-4";
const selectPadding = "pr-4";
const gameTypeSelectedColor = bgColors.green["400"]

interface GameOptionsProps {
    gameMode: GameModes;
}

export const GameOptions: FC<GameOptionsProps> = (props) => {
    const [gameType, setGameType] = useState<GameTypes | undefined>(undefined)
    const [,setGameTime] = useState<GameTimes | undefined>(GameTimes.Zero)
    const [,setGameIncrement] = useState<GameIncrements | undefined>(GameIncrements.Zero)
    const [,setGameDelay] = useState<GameDelays | undefined>(GameDelays.Zero)
    const [,setComputerDifficulty] = useState<ComputerDifficulties | undefined>(ComputerDifficulties.One)
    const [selectedColor,setSelectedColor] = useState<ColorOptions | undefined>(undefined)
    const [buttonAction, setButtonAction] = useState<ButtonActions>(ButtonActions.StartGame)

    useEffect(() => {
        switch (props.gameMode) {
            case GameModes.PlayOnline:
                setButtonAction(ButtonActions.FindGame)
                break;
            case GameModes.PlayAFriend:
            case GameModes.PlayComputer:
                setButtonAction(ButtonActions.StartGame)
                break;
        }
    }, [props.gameMode])

    return (
            <div className="mt-8 text-center sm:mt-0 sm:text-left w-full">
                <h3 className="text-2xl leading-6 font-medium text-gray-900 text-center" id="modal-headline">
                    Game Options
                </h3>
                <div className="mt-4 flex flex-col justify-center items-center">

                    {/* Game type */}
                    {props.gameMode === GameModes.PlayOnline ?
                        <ButtonGroup>
                            <ButtonGroup.Button
                                selected={gameType === GameTypes.RATED}
                                selectedColor={gameTypeSelectedColor}
                                onClick={() => setGameType(GameTypes.RATED)}
                            >
                                {GameTypes.RATED}
                            </ButtonGroup.Button>
                            <ButtonGroup.Button
                                selected={gameType === GameTypes.CASUAL}
                                selectedColor={gameTypeSelectedColor}
                                onClick={() => setGameType(GameTypes.CASUAL)}
                            >
                                {GameTypes.CASUAL}
                            </ButtonGroup.Button>
                        </ButtonGroup> : null}

                    {/* Time controls + difficulty */}
                    <div className={`flex ${optionPadding}`}>
                        <Select
                            className={selectPadding}
                            label="Time"
                            selectOptions={Object.values(GameTimes)}
                            onSelect={(value) => setGameTime(value as GameTimes)}
                        />
                        <Select
                            label="Increment"
                            selectOptions={Object.values(GameIncrements)}
                            onSelect={(value) => setGameIncrement(value as GameIncrements)}
                        />
                    </div>

                    {[GameModes.PlayAFriend, GameModes.PlayComputer].includes(props.gameMode) ?
                        <div className={`flex ${optionPadding}`}>
                            <Select
                                className={selectPadding}
                                label="Delay"
                                selectOptions={Object.values(GameDelays)}
                                onSelect={(value) => setGameDelay(value as GameDelays)}
                            />
                            {props.gameMode === GameModes.PlayComputer ?
                            <Select
                                label="Difficulty"
                                selectOptions={Object.values(ComputerDifficulties)}
                                onSelect={(value) => setComputerDifficulty(value as ComputerDifficulties)}
                            /> : null}
                        </div> : null}

                    {/* Color selection */}
                    <ButtonGroup className={`${optionPadding}`}>
                        <ButtonGroup.Button
                            style={pieceButtonStyle}
                            selected={selectedColor === ColorOptions.White}
                                selectedColor={gameTypeSelectedColor}
                            onClick={() => setSelectedColor(ColorOptions.White)}
                        >
                            <WhitePiece
                                style={pieceStyle}
                            />
                        </ButtonGroup.Button>
                        <ButtonGroup.Button
                            style={pieceButtonStyle}
                            selected={selectedColor === ColorOptions.Random}
                                selectedColor={gameTypeSelectedColor}
                            onClick={() => setSelectedColor(ColorOptions.Random)}
                        >
                            <SplitPiece
                                style={pieceStyle}
                            />
                        </ButtonGroup.Button>
                        <ButtonGroup.Button
                            style={pieceButtonStyle}
                            selected={selectedColor === ColorOptions.Black}
                                selectedColor={gameTypeSelectedColor}
                            onClick={() => setSelectedColor(ColorOptions.Black)}
                        >
                            <BlackPiece
                                style={pieceStyle}
                            />
                        </ButtonGroup.Button>
                    </ButtonGroup>

                    {/* Action button */}
                    <Button
                        className="mt-8"
                        roundSides
                        bgColor={
                            buttonAction === ButtonActions.FindGame ?
                                bgColors.yellow["300"] :
                                bgColors.green["400"]
                        }
                    >
                        {buttonAction}
                    </Button>
                </div>
            </div>
    )
}