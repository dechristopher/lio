import { ValidColors } from "@client/app/[rid]/Board";
import { PlayerColor } from "@client/proto/ws_pb";
import classNames from "classnames";
import React, { FC } from "react";
import { Piece, PieceType } from "../Piece/Piece";
import styles from "./PromotionModal.module.scss";

export enum File {
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
	file: File;
	playerColor: ValidColors;
	onPieceSelection: (piece: PromoPiece) => void;
}

const PromotionModal: FC<PromotionModalProps> = (props) => {
	if (!props.open) {
		return null;
	}

	function getFile(): string {
		if (props.playerColor === PlayerColor.WHITE) {
			switch (props.file) {
				case File.A:
					return styles.columnOne;
				case File.B:
					return styles.columnTwo;
				case File.C:
					return styles.columnThree;
				case File.D:
					return styles.columnFour;
			}
		} else {
			switch (props.file) {
				case File.A:
					return styles.columnFour;
				case File.B:
					return styles.columnThree;
				case File.C:
					return styles.columnTwo;
				case File.D:
					return styles.columnOne;
			}
		}
	}

	return (
		<div className={styles.container}>
			<div className={styles.promoShade} />
			<div
				className={classNames([styles.box, styles.rowOne, getFile()])}
				onClick={() => props.onPieceSelection(PromoPiece.QUEEN)}
			>
				<Piece
					pieceType={PieceType.QUEEN}
					pieceColor={props.playerColor}
				/>
			</div>
			<div
				className={classNames([styles.box, styles.rowTwo, getFile()])}
				onClick={() => props.onPieceSelection(PromoPiece.ROOK)}
			>
				<Piece
					pieceType={PieceType.ROOK}
					pieceColor={props.playerColor}
				/>
			</div>
			<div
				className={classNames([styles.box, styles.rowThree, getFile()])}
				onClick={() => props.onPieceSelection(PromoPiece.BISHOP)}
			>
				<Piece
					pieceType={PieceType.BISHOP}
					pieceColor={props.playerColor}
				/>
			</div>
			<div
				className={classNames([styles.box, styles.rowFour, getFile()])}
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
