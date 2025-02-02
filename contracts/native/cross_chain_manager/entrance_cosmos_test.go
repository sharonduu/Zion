/*
 * Copyright (C) 2020 The poly network Authors
 * This file is part of The poly network library.
 *
 * The  poly network  is free software: you can redistribute it and/or modify
 * it under the terms of the GNU Lesser General Public License as published by
 * the Free Software Foundation, either version 3 of the License, or
 * (at your option) any later version.
 *
 * The  poly network  is distributed in the hope that it will be useful,
 * but WITHOUT ANY WARRANTY; without even the implied warranty of
 * MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
 * GNU Lesser General Public License for more details.
 * You should have received a copy of the GNU Lesser General Public License
 * along with The poly network .  If not, see <http://www.gnu.org/licenses/>.
 */
package cross_chain_manager

import (
	"fmt"

	"github.com/ethereum/go-ethereum/accounts/abi"
	ccmcom "github.com/ethereum/go-ethereum/contracts/native/cross_chain_manager/common"

	synccom "github.com/ethereum/go-ethereum/contracts/native/header_sync/cosmos"

	"crypto/ecdsa"
	"encoding/hex"
	"encoding/json"
	"math/big"
	"reflect"

	ethcommon "github.com/ethereum/go-ethereum/common"

	"github.com/polynetwork/poly/common"

	"github.com/ethereum/go-ethereum/contracts/native"

	scom "github.com/ethereum/go-ethereum/contracts/native/header_sync/common"
	// scomcc "github.com/ethereum/go-ethereum/contracts/native/cross_chain_manager/common"

	"github.com/ethereum/go-ethereum/contracts/native/header_sync/zilliqa"

	cosmoscc "github.com/ethereum/go-ethereum/contracts/native/cross_chain_manager/cosmos"

	"github.com/ethereum/go-ethereum/contracts/native/utils"

	"github.com/ethereum/go-ethereum/contracts/native/governance/side_chain_manager"

	"github.com/ethereum/go-ethereum/core/rawdb"
	"github.com/ethereum/go-ethereum/core/state"

	"strings"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/ethereum/go-ethereum/contracts/native/governance"
	"github.com/ethereum/go-ethereum/contracts/native/governance/neo3_state_manager"
	"github.com/ethereum/go-ethereum/contracts/native/governance/node_manager"
	"github.com/ethereum/go-ethereum/contracts/native/governance/relayer_manager"
	"github.com/ethereum/go-ethereum/contracts/native/header_sync"

	"github.com/ethereum/go-ethereum/crypto"
	cstates "github.com/polynetwork/poly/core/states"
)
 
 const (
	 SUCCESS = iota
	 HEADER_NOT_EXIST
	 PROOF_FORMAT_ERROR
	 VERIFY_PROOT_ERROR
	 TX_HAS_COMMIT
	 UNKNOWN
 )
 
 func typeOfError(e error) int {
	 if e == nil {
		 return SUCCESS
	 }
	 errDesc := e.Error()
	 if strings.Contains(errDesc, "GetHeaderByHeight, height is too big") {
		 return HEADER_NOT_EXIST
	 } else if strings.Contains(errDesc, "unmarshal proof error:") {
		 return PROOF_FORMAT_ERROR
	 } else if strings.Contains(errDesc, "verify proof value hash failed") {
		 return VERIFY_PROOT_ERROR
	 } else if strings.Contains(errDesc, "check done transaction error:checkDoneTx, tx already done") {
		 return TX_HAS_COMMIT
	 }
	 return UNKNOWN
 }

var (
	sdb  *state.StateDB
	acct *ecdsa.PublicKey
	caller ethcommon.Address
	contractRef *native.ContractRef
)

func init() {
	governance.InitGovernance()
	header_sync.InitHeaderSync()
	InitCrossChainManager()
	
	neo3_state_manager.InitNeo3StateManager()
	node_manager.InitNodeManager()
	relayer_manager.InitRelayerManager()
	side_chain_manager.InitSideChainManager()

	db := rawdb.NewMemoryDatabase()
	sdb, _ = state.New(ethcommon.Hash{}, state.NewDatabase(db), nil)

	cacheDB := (*state.CacheDB)(sdb)
	

	blockNumber := big.NewInt(1)
	key, _ := crypto.GenerateKey()
	acct = &key.PublicKey
	caller = crypto.PubkeyToAddress(*acct)
	putPeerMapPoolAndView(cacheDB)
	contractRef = native.NewContractRef(sdb, caller, caller, blockNumber, ethcommon.Hash{}, 60000000, nil)
}

func putPeerMapPoolAndView(db *state.CacheDB) {
	/* key, _ := crypto.GenerateKey()
	acct = &key.PublicKey

	caller = crypto.PubkeyToAddress(*acct) */

	peerPoolMap := new(node_manager.PeerPoolMap)
	peerPoolMap.PeerPoolMap = make(map[string]*node_manager.PeerPoolItem)
	pkStr := hex.EncodeToString(crypto.FromECDSAPub(acct))
	peerPoolMap.PeerPoolMap[pkStr] = &node_manager.PeerPoolItem{
		Index:      uint32(0),
		PeerPubkey: pkStr,
		Address:    crypto.PubkeyToAddress(*acct),
		Status:     node_manager.ConsensusStatus,
	}
	view := uint32(0)
	viewBytes := utils.GetUint32Bytes(view)
	sink := common.NewZeroCopySink(nil)
	peerPoolMap.Serialization(sink)
	db.Put(utils.ConcatKey(utils.NodeManagerContractAddress, []byte(node_manager.PEER_POOL), viewBytes), cstates.GenRawStorageItem(sink.Bytes()))

	sink.Reset()

	govView := node_manager.GovernanceView{
		View: view,
	}
	govView.Serialization(sink)
	db.Put(utils.ConcatKey(utils.NodeManagerContractAddress, []byte(node_manager.GOVERNANCE_VIEW)), cstates.GenRawStorageItem(sink.Bytes()))
}

 
func RegisterSideChainManager(contractRef *native.ContractRef, chainId uint64) {
	param := new(side_chain_manager.RegisterSideChainParam)
	param.BlocksToWait = 4
	param.ChainId = chainId
	param.Name = "mychain"
	param.Router = 3
	param.Address = caller

	extraInfo  := zilliqa.ExtraInfo{NumOfGuardList: 1}
	b, _ := json.Marshal(extraInfo)
	param.ExtraInfo = b

	input, err := utils.PackMethodWithStruct(side_chain_manager.GetABI(), side_chain_manager.MethodRegisterSideChain, param)
	if err != nil {
		panic(err)
	}

	_, _, err = contractRef.NativeCall(caller, utils.SideChainManagerContractAddress, input)

	if err != nil {
		// panic(err)
	}
}

func NewNative(name string, param interface{}) (*native.NativeContract, error) {
	if scom.ABI == nil {
		scom.ABI = scom.GetABI()
	}
	if ccmcom.ABI == nil {
		ccmcom.ABI = ccmcom.GetABI()
	}

	var abi *abi.ABI
	var chainName string
	if name == ccmcom.MethodImportOuterTransfer {
		abi = ccmcom.ABI 
		chainName = "SourceChainID"
	} else {
		abi = scom.ABI
		chainName = "ChainID"
	}

	input, err := utils.PackMethodWithStruct(abi, name, param)
	if err != nil {
		return nil, err
	}

	

	contractRef.PushContext(&native.Context{
		Caller:          caller,
		ContractAddress: utils.CrossChainManagerContractAddress,
		Payload:         input,
	})

	c := native.NewNativeContract(sdb, contractRef)

	chainId := reflect.Indirect(reflect.ValueOf(param)).FieldByName(chainName).Uint()
	RegisterSideChainManager(contractRef, uint64(chainId))
	
    /*
	 input, err := utils.PackMethodWithStruct(scom.GetABI(), name, param)
	 if err != nil {
		 return nil, err
	 }

	 ret, leftOverGas, err := contractRef.NativeCall(ethcommon.Address{}, utils.HeaderSyncContractAddress, input)
	 if err != nil {
		 return nil, err
	 }
	 fmt.Printf("ret: %s, gas: %d", hex.EncodeToString(ret), leftOverGas) */
	return c, nil

	//result, err := utils.PackOutputs(header_sync.ABI, header_sync.MethodSyncBlockHeader, true)
	//_ = result
}
 
 func TestProofHandle(t *testing.T) {
	 cosmosHeaderSync := synccom.NewCosmosHandler()
	 cosmosProofHandler := cosmoscc.NewCosmosHandler()
	
	 {
		 header158265, _ := hex.DecodeString("0aaa020a02080a120963632d636f736d6f7318b9d409220b08f4caa0f80510938acd582a480a207923c1a8915f5cada506fea0546448e3efbe020fc753c00b21b1a64cc3ce80df12240801122066e362ca5f6ca4f9f8018be4ceb2a496e7a9f610b5ca667b7dbcf71a58b76928322061f85cd26b4e0a1c67ea44c01952b1bb20fa4fa3be03dbee3b4c6f21a791302742202ba25d7ae03b9152a4d8c4ab317ddb7bb3fcb834384ced538262e25834e6dcf14a202ba25d7ae03b9152a4d8c4ab317ddb7bb3fcb834384ced538262e25834e6dcf15220048091bc7ddc283f77bfbf91d73c44da58c3df8a9cbc867405d8b7f3daada22f5a202e63b1a0d5253c4ac9155078c8af0a1143e141f76b30bf02e877d50ffb493bbd72147f6abffe3fcf4afe3ce80e3080b749e170edbd24128c0308b9d4091a480a200b4a771c5506d5f860cc6f382fb4b0595aa857f92770419ad60a41156b19a47f1224080112203e315512893270ae79fdcdb53c210ded93815246a4e09baf6c2dfef5a26fd18e2268080212143a91887425aa1f560e2badd6e538d4baf7fa00501a0c08f9caa0f80510d89f82ec012240ba168ad6e1338bead146843cf7f8aad498cc8380c4bfea0ded39a9f4d9c7dd8f81f1f74bca536248de657551acc7912ab9d716669e728f46267007decee1810b2268080212147f6abffe3fcf4afe3ce80e3080b749e170edbd241a0c08f9caa0f80510e7b48eff012240512f3802e7381917d0dc99a12e500d8e562e4ac9e4751873eee99361277fe08773956a58fe3897b9a698e8acaa131a2903f63c20f319326ace787df0ffc5cc03226808021214e069c1227791131227fc946bee54eec2a39e191a1a0c08f9caa0f8051087a6bbed0122401b0bed5bcd5e9802815f60632d3bbc05b312059f8ae6034e32c2645a420fa711d162360f48212ca322458ea58cdca00367755aca03d8f3a1662058d6affa90001a4a0a143a91887425aa1f560e2badd6e538d4baf7fa005012251624de6420166388e0880ac8085074b64568310429f81252b6a93c05e1194a3108b2563d341864209cffffffffffffffff011a4a0a147f6abffe3fcf4afe3ce80e3080b749e170edbd2412251624de6420de052c42d0dc18e1bc64dc002a19214d537b83ae80afa9893a98352a7f2f025d1864209cffffffffffffffff011a420a14e069c1227791131227fc946bee54eec2a39e191a12251624de642095d57297855fdfb0e90a9193ce08c35d5eca6f555a4865803ba7ab4493b696c0186420c801")
		 param := new(scom.SyncGenesisHeaderParam)
		 param.ChainID = 5
		 param.GenesisHeader = header158265
		 sink := common.NewZeroCopySink(nil)
		 param.Serialization(sink)
 
		 native, err := NewNative(scom.MethodSyncGenesisHeader, param)
		 if err != nil {
			 fmt.Printf("%v", err)
		 }
		 err = cosmosHeaderSync.SyncGenesisHeader(native)
		 assert.NoError(t, err)
		 assert.Equal(t, SUCCESS, typeOfError(err))
	 }
	 {
		 param := new(ccmcom.EntranceParam)
		 header158266, _ := hex.DecodeString("0aab020a02080a120963632d636f736d6f7318bad409220c08f9caa0f8051087a6bbed012a480a200b4a771c5506d5f860cc6f382fb4b0595aa857f92770419ad60a41156b19a47f1224080112203e315512893270ae79fdcdb53c210ded93815246a4e09baf6c2dfef5a26fd18e3220574b32fd389adcc8859daba476617fd8a7b01beb71569d6d4d9b0fb2cea52fc142202ba25d7ae03b9152a4d8c4ab317ddb7bb3fcb834384ced538262e25834e6dcf14a202ba25d7ae03b9152a4d8c4ab317ddb7bb3fcb834384ced538262e25834e6dcf15220048091bc7ddc283f77bfbf91d73c44da58c3df8a9cbc867405d8b7f3daada22f5a203de956dd11723d5156d5f1a5cd699ea0f90e12a0226cfb1d9ba9370a997b1cf37214e069c1227791131227fc946bee54eec2a39e191a128c0308bad4091a480a20df4b9fb65ad659fbb30c19d95db808facee2d8bd0f8813a00c1700008f5fdc251224080112201bace219166e7795bc43b46ce13763a48a428a56e5e77f61d60a5d57557b1a612268080212143a91887425aa1f560e2badd6e538d4baf7fa00501a0c08fecaa0f80510a9a78a990322402f18580f66f09a9ace3a9603b548d44d5a0c6bb1a0cf1e13dea8da33fd54cddcb5b3e8fe3183f55b1d41f38a974679341b9cbb620f6facb93221a7070154fa072268080212147f6abffe3fcf4afe3ce80e3080b749e170edbd241a0c08fecaa0f80510bbc1f6aa03224041689ac9485674d15b668b505ffd068d314302213fb0c65cd18ecc56273f9e46ac29b644c638784b2eaf192b09940519ea336a34357d23f5321005962dafe40f226808021214e069c1227791131227fc946bee54eec2a39e191a1a0c08fecaa0f80510c3d0939a032240bb9afa5da243f9d31120f9cde6ebf19f4ab3ea17f93136d093379901820978c4c6f102694f6e8e45923b129531aeb2e707944972d9ae8442816a879d5b2bf10b1a3f0a143a91887425aa1f560e2badd6e538d4baf7fa005012251624de6420166388e0880ac8085074b64568310429f81252b6a93c05e1194a3108b2563d3418641a3f0a147f6abffe3fcf4afe3ce80e3080b749e170edbd2412251624de6420de052c42d0dc18e1bc64dc002a19214d537b83ae80afa9893a98352a7f2f025d18641a3f0a14e069c1227791131227fc946bee54eec2a39e191a12251624de642095d57297855fdfb0e90a9193ce08c35d5eca6f555a4865803ba7ab4493b696c01864")
		 proof, _ := hex.DecodeString("0af8060a066961766c3a761221010cff2e4b056a7b60706a8b04a9644c0b3f64eb45b91207e3250fbf0b63fe2fea1aca06c8060ac5060a2c082c10c5e81418ead3092a20972ce346217e2816d6656a653139eb5da9df1b6aba887748805d8d427d1577720a2c082a10b5920918ead3092a20371ca08defc0d5398dc142a22728e0e70c75d7507dfb5bb0b00619821df061820a2c082610fbad0318ead3092a20779899f4f63ee075b9f9c91445852573b8f1625f06f6e0e280a54ee9c53a95ad0a2c082410c7d00118ead3092a20265b7e16940e4ddffbd6c58fc38a700a4677bd206092e57d6c032873947eabb00a2b082010e55418ead309222073b0934005c30dedd32076d4cde64d368f61e694d806c42dc2f11b57c98047330a2b081e10be2518ead30922204b8222f4e53f53aff819b4cde10e8d7e03b7f9bcee0a799fd81db26a04acda600a2b081c10f91018ead3092a20d6ecfd8bfb9e7fb5942f4fceb011c071c1dca484ef5cdbbb2dc43751b955bfa50a2b081810e00618ead3092a209f1ee19951fa6290176e3b2a01b89bc80300472455d2712622abba61abfd494c0a2b081410850318ead3092a20dce62197fa36c33fc9bbf34bc375f97505d1e4a9fbfef2ba276f9628b024e2f30a2b081210bc0118ead3092220a9b7297d892985030be8dbe7fe749819b0c18bdd470eec4aecbb3151532b8ee00a2a0810106218ead30922202499b57c97dca2cd39f6ada9e4ee9cf4a2cf08a058f939ab2d20ac0cbe9d74620a2a080e103518ead309222017a18df6ff2f05488f75e72f13e772246c433a548590c5eee68ee20c7d65db630a2a080c102118ead3092220f3f0446557d058f0174deb5be115a550a3c636c49782f1c1acbf6e6443139c8b0a2a080a101118ead3092a20c649987a077a2c94b15648c326a024f47301f047f138b00c209ed2d66f0a95780a2a0806100718ead30922209c5f8b85d3803d31a45526bde38e6e32885a60e5f06c0d7c6ccc2463117900310a2a0804100418ead3092a20e41c3ee9e92ade194864a9a57dfc046388850ab3948a42a22739495f3ad8b5bd0a2a0802100218ead3092220b5e03ff6caf94b3974801718848ebe4dd93b74a22185c037b65b6ba943193c491a490a21010cff2e4b056a7b60706a8b04a9644c0b3f64eb45b91207e3250fbf0b63fe2fea12200cff2e4b056a7b60706a8b04a9644c0b3f64eb45b91207e3250fbf0b63fe2fea18ead3090af3050a0a6d756c746973746f7265120363636d1adf05dd050ada050a0c0a02667412060a0408b9d4090a330a077374616b696e6712280a2608b9d4091220b1a7eb220985230ad9eba03c47f3bd2f50c97786f3d6b44e8da02bb5b52199130a2f0a03676f7612280a2608b9d409122060cd137c1962ecac616389d68034833f2921509c2d285aa5f5153997ce968a740a350a096c6f636b70726f787912280a2608b9d4091220d2f0be6c9705f890c8e36ccef34689ac12a97df0c3cf0ad47bb3a24ebc5de1c90a2f0a0361636312280a2608b9d40912209d61df98e62da7252fabd3d01c43a2817a5dbdc5ca0049ebb48c1a3905df028c0a300a046d61696e12280a2608b9d4091220b19cf098b43b3c7d54799fef0944aa95099f82ddc219fa0c8092e452a282bf410a320a06706172616d7312280a2608b9d40912207737db314fdce9b7f9c4465cef7907beba483ddd91ce8198110232d3e54ca5bd0a320a06737570706c7912280a2608b9d40912202ee92cfcc9864b3162a2e55cefaf01affde2ef8d5b1c5783ad65de0619865efd0a380a0c646973747269627574696f6e12280a2608b9d40912204756c628aab3ddcc79270d54a203a67b8ee29a7687d6d622eac94037126fa6c90a360a0a68656164657273796e6312280a2608b9d409122077d4041830be3efb87398e0cd9d498a79dd8ec64f61f11f0b546e3e0a0ac29550a300a046d696e7412280a2608b9d409122071910be7aac211a257acc9911163af5d2df5adcf44ed4ae74296a64a6dd474350a110a077570677261646512060a0408b9d4090a340a08736c617368696e6712280a2608b9d409122043e97c84e24fc993fc0cefdf997259780d3cf98ed6fe08ebdc558d4076a4dabd0a300a046274637812280a2608b9d409122005927576dd48ec703d3e7e19a953c4f11c4663c85442d5dd4d29d5c4b756e4090a2f0a0363636d12280a2608b9d409122087f352a3daf136c825bdbdcd8a0bd19aa43106b22307d4cc366f8aaece1c2c400a120a0865766964656e636512060a0408b9d409")
		 value, _ := hex.DecodeString("0a542f63636d2f2530312530432546462e4b2530356a253742253630706a253842253034254139644c25304225334664254542452542392531322530372545332532352530462542462530426325464525324625454112a9012042b9a0d08f76be124dcc3026a5f7fe228fada451c21184d654fffe662e7086530302987714f71b55ef55cedc91fd007f7a9ba386ec978f3aa8030000000000000014b7041bc96b15da728fdfc1c47cbfc687b845adeb06756e6c6f636b4a14000000000000000000000000000000000000000114f3b8a17f1f957f60c88f105e32ebff3f022e56a44500000000000000000000000000000000000000000000000000000000000000")
		 param.SourceChainID = 5
		 param.Height = 158266
		 param.Proof = proof
		 param.RelayerAddress = []byte{}
		 param.Extra = value
		 param.HeaderOrCrossChainMsg = header158266
		 sink := common.NewZeroCopySink(nil)
		 param.Serialization(sink)
 
		 native, err := NewNative(ccmcom.MethodImportOuterTransfer, param)
		 if err != nil {
			 fmt.Printf("%v", err)
		 }
		 _, err = cosmosProofHandler.MakeDepositProposal(native)
		 assert.NoError(t, err)
		 assert.Equal(t, SUCCESS, typeOfError(err))
	 }
 }
 