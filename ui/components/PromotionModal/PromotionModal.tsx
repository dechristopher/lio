import { PlayerColor } from "@client/proto/ws_pb";
import classNames from "classnames";
import React, { FC } from "react";
import { Piece, PieceType } from "../Piece/Piece";
import styles from "./PromotionModal.module.scss";

export enum BoardColumns {
	A = "a",
	B = "b",
	C = "c",
	D = "d",
}

// TODO make this an enum within the protobuf file
export enum PromoPiece {
	QUEEN = "q",
	ROOK = "r",
	BISHOP = "b",
	KNIGHT = "n",
}

interface PromotionModalProps {
	open: boolean;
	boardColumn: BoardColumns;
	playerColor: PlayerColor.BLACK | PlayerColor.WHITE;
	onPieceSelection: (piece: PromoPiece) => void;
}

const PromotionModal: FC<PromotionModalProps> = (props) => {
	if (!props.open) {
		return null;
	}

	function getColumn(): string {
		switch (props.boardColumn) {
			case BoardColumns.A:
				return styles.columnOne;
			case BoardColumns.B:
				return styles.columnTwo;
			case BoardColumns.C:
				return styles.columnThree;
			case BoardColumns.D:
				return styles.columnFour;
		}
	}

	return (
		<div className={styles.container}>
			<div className={styles.promoShade} />
			<div
				className={classNames([styles.box, styles.rowOne, getColumn()])}
				onClick={() => props.onPieceSelection(PromoPiece.QUEEN)}
			>
				<Piece
					pieceType={PieceType.QUEEN}
					pieceColor={props.playerColor}
				/>
			</div>
			<div
				className={classNames([styles.box, styles.rowTwo, getColumn()])}
				onClick={() => props.onPieceSelection(PromoPiece.ROOK)}
			>
				<Piece
					pieceType={PieceType.ROOK}
					pieceColor={props.playerColor}
				/>
			</div>
			<div
				className={classNames([
					styles.box,
					styles.rowThree,
					getColumn(),
				])}
				onClick={() => props.onPieceSelection(PromoPiece.BISHOP)}
			>
				<Piece
					pieceType={PieceType.BISHOP}
					pieceColor={props.playerColor}
				/>
			</div>
			<div
				className={classNames([
					styles.box,
					styles.rowFour,
					getColumn(),
				])}
				onClick={() => props.onPieceSelection(PromoPiece.KNIGHT)}
			>
				<Piece
					pieceType={PieceType.KNIGHT}
					pieceColor={props.playerColor}
				/>
			</div>
		</div>
	);
};

export default PromotionModal;
