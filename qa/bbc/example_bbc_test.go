package bbc

import (
	"fmt"
	"testing"

	"github.com/dabankio/bbrpc"
	"github.com/dabankio/wallet-core/bip39"
	"github.com/dabankio/wallet-core/core/bbc"
	"github.com/stretchr/testify/require"
)

// 演示BBC sdk一般性用法
// 警告: 不要在生产环境中直接使用注释中的助记词
func TestExampleBBC(t *testing.T) {
	r := require.New(t)
	const pass = "123"
	// 临时启动一个测试节点
	killFunc, jsonRPC, minerAddress := bbrpc.TesttoolRunServerAndBeginMint(t)
	defer killFunc() //释放节点

	var seed []byte
	var err error
	{ // 种子
		entropy, err := bip39.NewEntropy(128) // <<=== sdk 生成熵, 128-256 32倍数
		require.NoError(t, err)
		err = bip39.SetWordListLang(bip39.LangChineseSimplified) // <<=== sdk 设定助记词语言,参考语言常量
		require.NoError(t, err)
		mnemonic, err := bip39.NewMnemonic(entropy) // <<=== sdk 生成助记词
		require.NoError(t, err)
		fmt.Println("mnemonic:", mnemonic) //mnemonic: 旦 件 言 毫 树 名 当 氧 旨 弧 落 功
		seed = bip39.NewSeed(mnemonic, "") // <<=== sdk 获取种子，第二个参数相当于salt,生产后请始终保持一致
	}

	var key *bbc.KeyInfo
	{
		key, err = bbc.DeriveKey(seed, 0, 0, 0) // <<=== sdk 推导key （账号0，作为向外部转账使用，第0个地址）
		r.NoError(err)
		fmt.Println("key", key) //key {0066760c7374abb65611092edd3176b5545772ed61b3672e1888a78846cbe308 8b48882c4e4d61e242d0da2c3b0bf025f77f0b6fef37a4efab7e996baeb93d6d 1dmyvkbkbk5zaqvx46zqpy2vzywjz02sv5kdd0gq2c56mwb48925hfhpd}
	}

	registeredAssets := 12.34
	{ // 导入公钥
		_, err = jsonRPC.Importpubkey(key.PublicKey) // <<=== RPC 导入公钥
		r.NoError(err)
		r.NoError(bbrpc.Wait4balanceReach(minerAddress, 10, jsonRPC))
		_, err = jsonRPC.Sendfrom(bbrpc.CmdSendfrom{
			From: minerAddress, To: key.Address, Amount: registeredAssets,
		})
		r.NoError(err)
		r.NoError(bbrpc.Wait4balanceReach(key.Address, registeredAssets, jsonRPC))
	}

	outAmount := 2.3
	{ //创建交易、签名、广播、检查余额
		rawTX, err := jsonRPC.Createtransaction(bbrpc.CmdCreatetransaction{ // <<=== RPC 创建交易
			From: key.Address, To: minerAddress, Amount: outAmount,
		})
		r.NoError(err)

		rawTX = replaceTXVersion(*rawTX)

		deTx, err := bbc.DecodeTX(*rawTX) // <<=== sdk 反序列化交易
		r.NoError(err)
		fmt.Println("decoded tx", deTx) //decoded tx {"Version":1,"Typ":0,"Timestamp":1584952846,"LockUntil":0,"SizeIn":1,"Prefix":2,"Amount":1340000,"TxFee":100,"SizeOut":0,"SizeSign":0,"HashAnchor":"00000000c335f935650a427bf548242eac4e4a444e25691b47351e7945f4a8d4","Address":"10g06z2bmwb71n9xg9zsv4vzay86ab7avt6n97hm6ra2z3rsbrtc2ncer","Sign":""}

		signedTX, err := bbc.SignWithPrivateKey(*rawTX, "", key.PrivateKey) // <<=== sdk 使用私钥对交易进行签名
		r.NoError(err)

		_, err = jsonRPC.Sendtransaction(signedTX) // <<=== RPC 发送交易
		r.NoError(err)

		r.NoError(bbrpc.Wait4nBlocks(1, jsonRPC))

		bal, err := jsonRPC.Getbalance(nil, &key.Address) // <<=== RPC 查询余额
		r.NoError(err)
		r.Len(bal, 1)
		r.True(bal[0].Avail < registeredAssets-outAmount)
		fmt.Println("balance after send", bal[0]) //balance after send {1dmyvkbkbk5zaqvx46zqpy2vzywjz02sv5kdd0gq2c56mwb48925hfhpd 0.9899 0 0}
	}

	var delegateTemplateAddress string
	{ //准备dpos节点数据
		delegateAddr, ownerAddr := bbrpc.TAddr0, bbrpc.TAddr1
		tplAddr, err := jsonRPC.Addnewtemplate(bbrpc.AddnewtemplateParamDelegate{
			Delegate: delegateAddr.Pubkey,
			Owner:    ownerAddr.Address,
		})
		r.Nil(err)
		delegateTemplateAddress = *tplAddr
		fmt.Println("delegate tpl addr:", delegateTemplateAddress)
	}

	voteAmount := 9.8
	var voteTemplateAddress string
	{ //投票和赎回
		// 首先添加投票地址
		voteTemplateAddressP, err := jsonRPC.Addnewtemplate(bbrpc.AddnewtemplateParamVote{
			Delegate: delegateTemplateAddress,
			Owner:    key.Address,
		})
		r.NoError(err)
		voteTemplateAddress = *voteTemplateAddressP

		addrInfo, err := jsonRPC.Validateaddress(voteTemplateAddress)
		r.NoError(err)

		rawTX, err := jsonRPC.Createtransaction(bbrpc.CmdCreatetransaction{
			From:   key.Address,
			To:     voteTemplateAddress,
			Amount: voteAmount,
		})
		rawTX = replaceTXVersion(*rawTX)
		// fmt.Println("rawtx", *rawTX)

		deTx, err := bbc.DecodeTX(*rawTX) // <<=== sdk 反序列化交易
		r.NoError(err)
		fmt.Println("decoded tx", deTx) //decoded tx {"Version":1,"Typ":0,"Timestamp":1584952846,"LockUntil":0,"SizeIn":1,"Prefix":2,"Amount":1340000,"TxFee":100,"SizeOut":0,"SizeSign":0,"HashAnchor":"00000000c335f935650a427bf548242eac4e4a444e25691b47351e7945f4a8d4","Address":"10g06z2bmwb71n9xg9zsv4vzay86ab7avt6n97hm6ra2z3rsbrtc2ncer","Sign":""}

		signedTX, err := bbc.SignWithPrivateKey(*rawTX, addrInfo.Addressdata.Templatedata.Hex, key.PrivateKey) // <<=== sdk 使用私钥对交易进行签名,传入投票模版地址数据
		// fmt.Println("signed tx", signedTX)
		r.NoError(err)

		_, err = jsonRPC.Sendtransaction(signedTX) // <<=== RPC 发送交易
		r.NoError(err)

		r.NoError(bbrpc.Wait4nBlocks(1, jsonRPC))

		bal, err := jsonRPC.Getbalance(nil, &voteTemplateAddress) // <<=== RPC 查询余额
		r.NoError(err)
		r.Len(bal, 1)
		r.Equal(bal[0].Avail, voteAmount)
		fmt.Println("vote template balance", bal[0]) //balance after vote

		{ //赎回部分投票
			redeemAmount := 2.3
			tx2, err := jsonRPC.Createtransaction(bbrpc.CmdCreatetransaction{
				From:   voteTemplateAddress,
				To:     key.Address,
				Amount: redeemAmount,
			})
			r.NoError(err)
			tx2 = replaceTXVersion(*tx2)
			deTx, err = bbc.DecodeTX(*tx2)
			r.NoError(err)
			signedTX, err = bbc.SignWithPrivateKey(*tx2, addrInfo.Addressdata.Templatedata.Hex, key.PrivateKey)
			r.NoError(err)
			_, err = jsonRPC.Sendtransaction(signedTX) // <<=== RPC 发送交易
			r.NoError(err)

			r.NoError(bbrpc.Wait4nBlocks(1, jsonRPC))
			bal, err := jsonRPC.Getbalance(nil, &voteTemplateAddress) // <<=== RPC 查询余额
			r.NoError(err)
			r.Len(bal, 1)
			r.True(bal[0].Avail < voteAmount - redeemAmount)
			fmt.Println("vote template balance after redeem", bal[0]) //balance after vote
		}
	}

	{ //直接使用私钥的场景
		key, err = bbc.ParsePrivateKey(key.PrivateKey) // <<=== sdk 解析私钥为公钥/地址
		r.NoError(err)
	}
}

func replaceTXVersion(rawtx string) *string {
	_dposTx := "ffff" + rawtx[4:] //dpos测试链需要修改tx version，主网不需要该环节
	return &_dposTx
}
