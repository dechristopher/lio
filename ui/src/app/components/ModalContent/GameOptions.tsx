import React, {FC, useEffect, useState} from "react";
import {ButtonGroup} from "@components/ButtonGroup/ButtonGroup";
import {Select} from "@components/Select/Select";
import WhitePiece from "@assets/images/pieces/White.svg";
import SplitPiece from "@assets/images/pieces/Split.svg";
import BlackPiece from "@assets/images/pieces/Black.svg";
import {bgColors} from "@utils/styles/colors";
import {Button} from "@components/Button/Button";
import {ModalContextActions, useModalContext} from "@app/contexts/ModalContext";
import {PreGame} from "@app/components/ModalContent/PreGame";
import {RatedGame} from "@app/queries/FetchRatedPools";
import {ColorOptions, GameModes, GameTypes, Times} from "@utils/constants";

const GameTimes = [
    Times.Zero,
    Times.Five,
    Times.Fifteen,
    Times.Thirty,
    Times.OneMin,
    Times.ThreeMin,
    Times.FiveMin,
    Times.TenMin
]

const GameIncrements = [
    Times.Zero,
    Times.One,
    Times.Three,
    Times.Five,
    Times.Ten,
    Times.Fifteen,
    Times.Thirty
]

const GameDelays = [
    Times.Zero,
    Times.One,
    Times.Three,
    Times.Five,
    Times.Ten,
    Times.Fifteen,
    Times.Thirty
]

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
    const [, modalDispatch] = useModalContext()

    const [gameType, setGameType] = useState<GameTypes | undefined>(undefined)
    const [gameTime, setGameTime] = useState<Times>(GameTimes[0])
    const [gameIncrement, setGameIncrement] = useState<Times>(GameIncrements[0])
    const [gameDelay, setGameDelay] = useState<Times>(GameDelays[0])
    const [, setComputerDifficulty] = useState<ComputerDifficulties | undefined>(ComputerDifficulties.One)
    const [selectedColor, setSelectedColor] = useState<ColorOptions | undefined>(undefined)
    const [buttonAction, setButtonAction] = useState<ButtonActions>(ButtonActions.StartGame)
    const [gameSettings, setGameSettings] = useState<RatedGame>({
        name: formatGameTime(gameTime, gameIncrement, gameDelay),
        group: "",
        time: {
            t: gameTime,
            i: gameIncrement,
            d: gameDelay
        }
    })

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

    useEffect(() => {
        setGameSettings({
            ...gameSettings,
            name: formatGameTime(gameTime, gameIncrement, gameDelay),
            time: {
                t: gameTime,
                i: gameIncrement,
                d: gameDelay
            }
        })
    }, [gameTime, gameIncrement, gameDelay])

    // prevents the user from proceeding if they need to choose required options
    const isButtonDisabled = () => {
        if (gameTime === Times.Zero && gameIncrement === Times.Zero && gameDelay === Times.Zero) {
            return true
        } else if (props.gameMode === GameModes.PlayOnline) {
            return gameType === undefined;
        } else if ([GameModes.PlayAFriend, GameModes.PlayComputer].includes(props.gameMode)) {
            return selectedColor === undefined
        }

        return true
    }

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
                        onSelect={(value) => setGameTime(value as Times)}
                    />
                    <Select
                        label="Increment"
                        selectOptions={Object.values(GameIncrements)}
                        onSelect={(value) => setGameIncrement(value as Times)}
                    />
                </div>

                {[GameModes.PlayAFriend, GameModes.PlayComputer].includes(props.gameMode) ?
                    <div className={`flex ${optionPadding}`}>
                        <Select
                            className={selectPadding}
                            label="Delay"
                            selectOptions={Object.values(GameDelays)}
                            onSelect={(value) => setGameDelay(value as Times)}
                        />
                        {props.gameMode === GameModes.PlayComputer ?
                            <Select
                                label="Difficulty"
                                selectOptions={Object.values(ComputerDifficulties)}
                                onSelect={(value) => setComputerDifficulty(value as ComputerDifficulties)}
                            /> : null}
                    </div> : null}

                {/* Color selection */}
                {props.gameMode === GameModes.PlayOnline ? null :
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
                    </ButtonGroup>}

                {/* Action button */}
                <Button
                    roundSides
                    className="mt-8 px-8 py-2"
                    disabled={isButtonDisabled()}
                    onClick={() => {
                        modalDispatch({
                            type: ModalContextActions.SetContent,
                            payload: <PreGame
                                gameMode={props.gameMode}
                                gameType={gameSettings}
                            />
                        })
                    }}
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

/**
 * Formats the current time settings to `<minutes>:<seconds> + <increment> ~<delay>`.
 *
 * @param {Times} gameTime - game time
 * @param {Times} gameIncrement - game increment
 * @param {Times} gameDelay - game delay
 * @returns {string} formatted game time settings
 *
 * @example
 * formatGameTime(30, 3, 0)
 */
const formatGameTime = (
    gameTime: Times,
    gameIncrement: Times,
    gameDelay: Times
): string => {
    const mins = Math.floor(gameTime / 60);
    const seconds = gameTime % 60;

    // if seconds is greater than 10, show as is
    // else prepend a 0 to the front
    const time = seconds >= 10 ? `${mins}:${seconds}` : `${mins}:0${seconds}`

    if (gameDelay === Times.Zero) {
        return `${time} + ${gameIncrement}`
    } else if (gameIncrement === Times.Zero) {
        return `${time} ~${gameDelay}`
    }

    return `${time} + ${gameIncrement} ~${gameDelay}`
}